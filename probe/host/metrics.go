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

package host

import (
	"github.com/megaease/easeprobe/global"
	"github.com/megaease/easeprobe/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics is the metrics for host probe
type metrics struct {
	CPU    *prometheus.GaugeVec
	Memory *prometheus.GaugeVec
	Disk   *prometheus.GaugeVec
}

// newMetrics create the host metrics
func newMetrics(subsystem, name string) *metrics {
	namespace := global.GetEaseProbe().Name
	return &metrics{
		CPU: metric.NewGauge(namespace, subsystem, name, "cpu",
			"CPU Usage", []string{"host", "state"}),
		Memory: metric.NewGauge(namespace, subsystem, name, "memory",
			"Memory Usage", []string{"host", "state"}),
		Disk: metric.NewGauge(namespace, subsystem, name, "disk",
			"Disk Usage", []string{"host", "disk", "state"}),
	}
}
