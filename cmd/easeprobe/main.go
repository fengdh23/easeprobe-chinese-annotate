/*
 * Copyright (c) 2022, MegaEase
 * All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"flag"
	"os"
	"time"

	"github.com/megaease/easeprobe/conf"
	"github.com/megaease/easeprobe/global"
	"github.com/megaease/easeprobe/notify"
	"github.com/megaease/easeprobe/probe"
	"github.com/megaease/easeprobe/web"

	"github.com/go-co-op/gocron"
	log "github.com/sirupsen/logrus"
)

func getEnvOrDefault(key, defaultValue string) string {
	// value, ok := os.LookupEnv(key); 赋值、初始化表达式 ; 分号后面是 bool值的判断表达式
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

// 程序入口，最底层的组件
func main() {
	// 定义命令行变量， 环境变量 PROBE_DRY 和 配置文件名 PROBE_CONFIG
	//TODO Dry 是什么意思？ 目前仅仅是 log 记录。--> DryNotify just log the notification message
	dryNotify := flag.Bool("d", os.Getenv("PROBE_DRY") == "true", "dry notification mode")
	yamlFile := flag.String("f", getEnvOrDefault("PROBE_CONFIG", "config.yaml"), "configuration file")
	// 解析命令行
	flag.Parse()
	// conf　对象的公开方法 New 从配置文件读取 返回 conf 配置对象
	conf, err := conf.New(yamlFile)
	if err != nil {
		log.Fatalln("Fatal: Cannot read the YAML configuration file!")
		os.Exit(-1)
	}
	// 提前定义 defer , 程序退出后关闭日志
	defer conf.CloseLogFile()

	// if dry notification mode is specificed in command line, overwrite the configuration
	if *dryNotify {
		//TODO 类似 java builder链式调用，返回原对象。但是源码没看到，如何实现的？
		conf.Settings.Notify.Dry = *dryNotify
	}

	if conf.Settings.Notify.Dry {
		log.Infoln("Dry Notification Mode...")
	}

	// Probers 所有探测者(切片)
	probers := conf.AllProbers()

	// Notification 所有通知提醒者(切片)
	notifies := conf.AllNotifiers()
	//TODO 是否完成的一个标识，bool 类型， channel 同步?
	done := make(chan bool)
	// 运行逻辑
	run(probers, notifies, done)

}

// 1) all of probers send the result to notify channel 结果发送到通知的通道
// 2) go through all of notification to notify the result.
// 3) send the SLA report
func run(probers []probe.Prober, notifies []notify.Notify, done chan bool) {
	// boolean 变量
	dryNotify := conf.Get().Settings.Notify.Dry

	// Create the Notification Channel 处理结果的通道
	notifyChan := make(chan probe.Result)

	// Configure the Probes 配置
	configProbers(probers, notifyChan)

	// Configure the Notifiers 配置
	configNotifiers(notifies)

	// Start the HTTP Server 4.27 才新增的 启动探测对象
	web.SetProbers(probers)
	// 启动服务
	web.Server()

	// Set the Cron Job for SLA Report 定时任务表达式
	if conf.Get().Settings.SLAReport.Schedule != conf.None {
		scheduleSLA(probers, notifies)
	} else {
		log.Info("No SLA Report would be sent!!")
	}

	// Watching the Probe Event...
	// 无限循环
	for {
		// 仅能用于channel发送和接收消息的语句，此语句运行期间是阻塞的；当 select中没有case语句的时候，会阻塞当前goroutine
		select {
		// 完成则返回
		case <-done:
			return
		// 从通道获取数据
		case result := <-notifyChan:
			// if the status has no change, no need notify
			if result.PreStatus == result.Status {
				log.Debugf("%s (%s) - Status no change [%s] == [%s], no notification.",
					result.Name, result.Endpoint, result.PreStatus, result.Status)
				continue
			}
			if result.PreStatus == probe.StatusInit && result.Status == probe.StatusUp {
				log.Debugf("%s (%s) - Initial Status [%s] == [%s], no notification.",
					result.Name, result.Endpoint, result.PreStatus, result.Status)
				continue
			}
			log.Infof("%s (%s) - Status changed [%s] ==> [%s]",
				result.Name, result.Endpoint, result.PreStatus, result.Status)
			// 遍历通知者
			for _, n := range notifies {
				if dryNotify {
					// log 记录，和 DryNotifyStat 区别
					n.DryNotify(result)
				} else {
					// 异步，各自实现的通知
					go n.Notify(result)
				}
			}
		}
	}

}

func configProbers(probers []probe.Prober, notifyChan chan probe.Result) {
	// 定义函数
	probeFn := func(p probe.Prober) {
		// 无限循环
		for {
			res := p.Probe()
			log.Debugf("%s: %s", p.Kind(), res.DebugJSON())
			//TODO <- 操作符， 标识数据的流向，数据放到通道里. 上下文交互？
			notifyChan <- res
			// 休息时间间隔
			time.Sleep(p.Interval())
		}
	}
	// 配置赋值
	gProbeConf := global.ProbeSettings{
		TimeFormat: conf.Get().Settings.TimeFormat,
		Interval:   conf.Get().Settings.Probe.Interval,
		Timeout:    conf.Get().Settings.Probe.Timeout,
	}
	log.Debugf("Global Probe Configuration: %+v", gProbeConf)
	// 遍历探测对象，每个对象先配置--> 执行探测函数
	for _, p := range probers {
		err := p.Config(gProbeConf)
		if err != nil {
			p.Result().Message = "Bad Configuration: " + err.Error()
			log.Errorf("error: %v", err)
			continue
		}

		p.Result().Message = "Good Configuration!"
		log.Infof("Ready to monitor(%s): %s - %s", p.Kind(), p.Result().Name, p.Result().Endpoint)
		go probeFn(p)
	}
}

func configNotifiers(notifies []notify.Notify) {
	gNotifyConf := global.NotifySettings{
		TimeFormat: conf.Get().Settings.TimeFormat,
		Retry:      conf.Get().Settings.Notify.Retry,
	}
	for _, n := range notifies {
		// 每个通知者的配置实现
		err := n.Config(gNotifyConf)
		if err != nil {
			log.Errorf("error: %v", err)
			continue
		}
		log.Infof("Successfully setup the notify channel: %s", n.Kind())
	}
}

func scheduleSLA(probers []probe.Prober, notifies []notify.Notify) {
	// 第三方表达式框架 gocron
	cron := gocron.NewScheduler(time.UTC)

	dryNotify := conf.Get().Settings.Notify.Dry

	SLAFn := func() {
		for _, n := range notifies {
			// stat 统计信息的通知
			if dryNotify {
				n.DryNotifyStat(probers)
			} else {
				log.Debugf("[%s] notifying the SLA...", n.Kind())
				go n.NotifyStat(probers)
			}
		}
		//获取下次执行的时间
		_, t := cron.NextRun()
		log.Infof("Next Time to send the SLA Report - %s", t.Format(conf.Get().Settings.TimeFormat))
	}

	if conf.Get().Settings.SLAReport.Debug {
		// debug 模式，每分钟执行一次SLAFn
		cron.Every(1).Minute().Do(SLAFn)
		log.Infoln("Preparing to send the  SLA report in every minute...")
	} else {
		// 读取配置的时间周期来执行 SLAFn
		time := conf.Get().Settings.SLAReport.Time
		switch conf.Get().Settings.SLAReport.Schedule {
		case conf.Daily:
			cron.Every(1).Day().At(time).Do(SLAFn)
			log.Infof("Preparing to send the daily SLA report at %s UTC time...", time)
		case conf.Weekly:
			cron.Every(1).Day().Sunday().At(time).Do(SLAFn)
			log.Infof("Preparing to send the weekly SLA report on Sunday at %s UTC time...", time)
		case conf.Monthly:
			cron.Every(1).MonthLastDay().At(time).Do(SLAFn)
			log.Infof("Preparing to send the monthly SLA report in last day at %s UTC time...", time)
		default:
			cron.Every(1).Day().At("00:00").Do(SLAFn)
			log.Warnf("Bad Scheduling! Preparing to send the daily SLA report at 00:00 UTC time...")
		}
	}
	// 需要熟悉 cron
	// 开始异步执行
	cron.StartAsync()
	// 获取下次执行的时间
	_, t := cron.NextRun()
	log.Infof("Next Time to send the SLA Report - %s", t.Format(conf.Get().Settings.TimeFormat))
}
