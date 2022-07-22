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

package web

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/megaease/easeprobe/conf"
	"github.com/megaease/easeprobe/global"
	"github.com/megaease/easeprobe/probe"
	"github.com/megaease/easeprobe/report"

	log "github.com/sirupsen/logrus"
)

var probers *[]probe.Prober

func slaHTML(w http.ResponseWriter, req *http.Request) {
	interval := conf.Get().Settings.HTTPServer.AutoRefreshTime
	refresh := fmt.Sprintf("%d", interval.Milliseconds())
	html := []byte(report.SLAHTML(*probers) + report.AutoRefreshJS(refresh))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}

func slaJSON(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte(report.SLAJSON(*probers)))
}

// SetProbers set the probers
func SetProbers(p []probe.Prober) {
	probers = &p
}

// Server is the http server
func Server() {
	// 获取全局配置  getAllConfig
	c := conf.Get()
	// 从全局配置获取 host 和 ip
	host := c.Settings.HTTPServer.IP
	port := c.Settings.HTTPServer.Port

	// Configure the http server
	if len(host) > 0 && net.ParseIP(host) == nil {
		host = global.DefaultHTTPServerIP
	}
	// 端口 string 转换为 int ,利用 strconv.Atoi()
	p, err := strconv.Atoi(port)
	// 端口值的范围控制，超出告警并赋值成默认值
	if err != nil || p <= 1024 || p > 65535 {
		log.Warnf("[Web] Invalid port number: %s, use the default value: %s", port, global.DefaultHTTPServerPort)
		port = global.DefaultHTTPServerPort
	} else {
		port = c.Settings.HTTPServer.Port
	}
	//  SLA是服务等级协议（Service Level Agreement) the SLA page -->
	// Configure the auto refresh time of the SLA page
	if c.Settings.HTTPServer.AutoRefreshTime == 0 {
		interval := global.DefaultProbeInterval
		// find the minimum probe interval 没有配置刷新间隔时间则遍历获取最小的 时间间隔，用于自动刷新。
		for p := range *probers {
			if interval > (*probers)[p].Interval() {
				interval = (*probers)[p].Interval()
			}
		}
		c.Settings.HTTPServer.AutoRefreshTime = interval
		log.Debugf("[Web] Auto refresh interval time: %s", interval)
	}

	// Start the http server
	go func() {
		// 自定义的html 格式的handler
		http.HandleFunc("/", slaHTML)
		// 自定义的json 格式的handler
		http.HandleFunc("/api/v1/sla", slaJSON)
		log.Infof("[Web] HTTP server is listening on %s:%s", host, port)
		// 不带处理器的 监听。The handler is usually nil, which means to use DefaultServeMux
		if err := http.ListenAndServe(host+":"+port, nil); err != nil {
			log.Errorf("[Web] HTTP server error: %s", err)
		}
	}()
}
