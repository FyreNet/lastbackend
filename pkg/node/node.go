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

package node

import (
	"context"
	"fmt"
	"github.com/lastbackend/lastbackend/pkg/node/ingress/ingress"
	"github.com/lastbackend/lastbackend/pkg/runtime/iri/iri"
	"os"
	"os/signal"
	"syscall"

	"github.com/lastbackend/lastbackend/pkg/node/runtime"
	"github.com/lastbackend/lastbackend/pkg/node/state"

	"github.com/lastbackend/lastbackend/pkg/api/client"
	"github.com/lastbackend/lastbackend/pkg/log"
	"github.com/lastbackend/lastbackend/pkg/node/envs"
	"github.com/lastbackend/lastbackend/pkg/node/events"
	"github.com/lastbackend/lastbackend/pkg/node/events/exporter"
	"github.com/lastbackend/lastbackend/pkg/node/http"
	"github.com/lastbackend/lastbackend/pkg/runtime/cni/cni"
	"github.com/lastbackend/lastbackend/pkg/runtime/cpi/cpi"
	"github.com/lastbackend/lastbackend/pkg/runtime/cri/cri"
	"github.com/lastbackend/lastbackend/pkg/runtime/csi/csi"
	"github.com/spf13/viper"
)

// Daemon - run node daemon
func Daemon() {

	var (
		sigs = make(chan os.Signal)
		done = make(chan bool, 1)
	)

	log.New(viper.GetInt("verbose"))
	log.Info("Start Node")

	criDriver := viper.GetString("runtime.cri.type")
	_cri, err := cri.New(criDriver, viper.GetStringMap(fmt.Sprintf("runtime.%s", criDriver)))
	if err != nil {
		log.Errorf("Cannot initialize cri: %v", err)
	}

	iriDriver := viper.GetString("runtime.iri.type")
	_iri, err := iri.New(iriDriver, viper.GetStringMap(fmt.Sprintf("runtime.%s", iriDriver)))
	if err != nil {
		log.Errorf("Cannot initialize iri: %v", err)
	}

	_cni, err := cni.New()
	if err != nil {
		log.Errorf("Cannot initialize cni: %v", err)
	}

	_cpi, err := cpi.New()
	if err != nil {
		log.Errorf("Cannot initialize cni: %v", err)
	}

	csis := viper.GetStringMap("runtime.csi")
	if csis != nil {
		for kind := range csis {
			si, err := csi.New(kind)
			if err != nil {
				log.Errorf("Cannot initialize sni: %s > %v", kind, err)
			}
			envs.Get().SetCSI(kind, si)
		}

	}

	envs.Get().SetDNS(viper.GetStringSlice("dns.ips"))

	st := state.New()
	envs.Get().SetState(st)
	envs.Get().SetCRI(_cri)
	envs.Get().SetIRI(_iri)
	envs.Get().SetCNI(_cni)
	envs.Get().SetCPI(_cpi)

	envs.Get().SetModeIngress(viper.GetBool("ingress.enable"))
	if envs.Get().GetModeIngress() {
		ing, err := ingress.New()
		if err != nil {
			log.Errorf("Cannot initialize iri: %v", err)
		}
		envs.Get().SetIngress(ing)
	}

	st.Node().Info = runtime.NodeInfo()
	st.Node().Status = runtime.NodeStatus()

	cfg := client.NewConfig()

	cfg.BearerToken = viper.GetString("token")

	if viper.IsSet("api.tls") && !viper.GetBool("api.tls.insecure") {
		cfg.TLS = client.NewTLSConfig()
		cfg.TLS.CertFile = viper.GetString("api.tls.cert")
		cfg.TLS.KeyFile = viper.GetString("api.tls.key")
		cfg.TLS.CAFile = viper.GetString("api.tls.ca")
	}

	endpoint := viper.GetString("api.uri")
	rest, err := client.New(client.ClientHTTP, endpoint, cfg)
	if err != nil {
		log.Fatalf("Init client err: %s", err)
	}

	if err != nil {
		log.Errorf("node:initialize client err: %s", err.Error())
		os.Exit(0)
	}

	n := rest.V1().Cluster().Node(st.Node().Info.Hostname)
	s := rest.V1()
	envs.Get().SetClient(n, s)

	e := exporter.NewExporter()
	e.SetDispatcher(events.Dispatcher)
	envs.Get().SetExporter(e)

	r := runtime.NewRuntime(context.Background())
	r.Restore()

	if err := r.Connect(context.Background()); err != nil {
		log.Fatalf("node:initialize: connect err %s", err.Error())
	}

	go r.Subscribe()

	e.Loop()
	r.Loop()

	go func() {
		opts := new(http.HttpOpts)
		opts.Insecure = viper.GetBool("node.tls.insecure")
		opts.CertFile = viper.GetString("node.tls.server_cert")
		opts.KeyFile = viper.GetString("node.tls.server_key")
		opts.CaFile = viper.GetString("node.tls.ca")

		if err := http.Listen(viper.GetString("node.host"), viper.GetInt("node.port"), opts); err != nil {
			log.Fatalf("Http server start error: %v", err)
		}
	}()

	// Handle SIGINT and SIGTERM.
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			select {
			case <-sigs:
				done <- true
				return
			}
		}
	}()

	<-done

	log.Info("Handle SIGINT and SIGTERM.")

	return
}
