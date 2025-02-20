// Copyright (c) 2019 OpenSDS Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package routers

import (
	"github.com/astaxie/beego"

	"github.com/opensds/opensds/pkg/api/controllers"
	"github.com/opensds/opensds/pkg/utils/constants"
)

func init() {

	// add router for metric api
	metricns :=
		beego.NewNamespace("/"+constants.APIVersion+"/:tenantId/metrics",

			// All operations of metrics can be used for both admin and users.
			beego.NSRouter("/", controllers.NewMetricsPortal(), "post:CollectMetrics;get:GetMetrics"),

			// All operations of file can be used for both admin and users.
			beego.NSRouter("/uploadconf", controllers.NewMetricsPortal(), "post:UploadConfFile"),
			beego.NSRouter("/downloadconf", controllers.NewMetricsPortal(), "get:DownloadConfFile"),

			beego.NSRouter("/urls", controllers.NewMetricsPortal(), "get:GetUrls"),
		)
	beego.AddNamespace(metricns)
}
