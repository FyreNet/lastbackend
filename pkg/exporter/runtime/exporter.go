//
// Last.Backend LLC CONFIDENTIAL
// __________________
//
// [2014] - [2018] Last.Backend LLC
// All Rights Reserved.
//
// NOTICE:  All information contained herein is, and remains
// the property of Last.Backend LLC and its suppliers,
// if any.  The intellectual and technical concepts contained
// herein are proprietary to Last.Backend LLC
// and its suppliers and may be covered by Russian Federation and Foreign Patents,
// patents in process, and are protected by trade secret or copyright law.
// Dissemination of this information or reproduction of this material
// is strictly forbidden unless prior written permission is obtained
// from Last.Backend LLC.
//

package runtime

import (
	"fmt"
	"github.com/lastbackend/lastbackend/pkg/distribution/types"
	"github.com/lastbackend/lastbackend/pkg/util/proxy"
	"github.com/lastbackend/lastbackend/pkg/util/system"
	"github.com/spf13/viper"
	"os"
)

func ExporterInfo() types.ExporterInfo {

	var (
		info = types.ExporterInfo{}
	)

	osInfo := system.GetOsInfo()
	hostname, err := os.Hostname()
	if err != nil {
		_ = fmt.Errorf("get hostname err: %s", err)
	}

	iface := viper.GetString("runtime.interface")
	ip, err := system.GetHostIP(iface)
	if err != nil {
		_ = fmt.Errorf("get ip err: %s", err)
	}

	info.Hostname = hostname
	info.InternalIP = ip
	info.OSType = osInfo.GoOS
	info.OSName = fmt.Sprintf("%s %s", osInfo.OS, osInfo.Core)
	info.Architecture = osInfo.Platform

	return info
}

func ExporterStatus() types.ExporterStatus {

	var state = types.ExporterStatus{}

	iface := viper.GetString("runtime.interface")
	ip, err := system.GetHostIP(iface)
	if err != nil {
		_ = fmt.Errorf("get ip err: %s", err)
	}

	lp := uint16(viper.GetInt("exporter.listener.port"))
	if lp == 0 {
		lp = proxy.DefaultPort
	}

	hp := uint16(viper.GetInt("exporter.http.port"))
	if hp == 0 {
		hp = proxy.DefaultPort
	}

	state.Ready = true
	state.Listener.Port = lp
	state.Listener.IP = ip

	state.Http.IP = ip
	state.Http.Port = hp

	return state
}
