package services

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/ansible/receptor/pkg/netceptor"
	"github.com/ansible/receptor/pkg/utils"
	"github.com/ghjm/cmdline"
	"github.com/spf13/viper"
)

// UnixProxyServiceInbound listens on a Unix socket and forwards connections over the Receptor network.
func UnixProxyServiceInbound(s *netceptor.Netceptor, filename string, permissions os.FileMode,
	node string, rservice string, tlscfg *tls.Config,
) error {
	uli, lock, err := utils.UnixSocketListen(filename, permissions)
	if err != nil {
		return fmt.Errorf("error opening Unix socket: %s", err)
	}
	go func() {
		defer lock.Unlock()
		for {
			uc, err := uli.Accept()
			if err != nil {
				s.Logger.Error("Error accepting Unix socket connection: %s", err)

				return
			}
			go func() {
				qc, err := s.Dial(node, rservice, tlscfg)
				if err != nil {
					s.Logger.Error("Error connecting on Receptor network: %s", err)

					return
				}
				utils.BridgeConns(uc, "unix socket service", qc, "receptor connection", s.Logger)
			}()
		}
	}()

	return nil
}

// UnixProxyServiceOutbound listens on the Receptor network and forwards the connection via a Unix socket.
func UnixProxyServiceOutbound(s *netceptor.Netceptor, service string, tlscfg *tls.Config, filename string) error {
	qli, err := s.ListenAndAdvertise(service, tlscfg, map[string]string{
		"type":     "Unix Proxy",
		"filename": filename,
	})
	if err != nil {
		return fmt.Errorf("error listening on Receptor network: %s", err)
	}
	go func() {
		for {
			qc, err := qli.Accept()
			if err != nil {
				s.Logger.Error("Error accepting connection on Receptor network: %s\n", err)

				return
			}
			uc, err := net.Dial("unix", filename)
			if err != nil {
				s.Logger.Error("Error connecting via Unix socket: %s\n", err)

				continue
			}
			go utils.BridgeConns(qc, "receptor service", uc, "unix socket connection", s.Logger)
		}
	}()

	return nil
}

// unixProxyInboundCfg is the cmdline configuration object for a Unix socket inbound proxy.
type UnixProxyInboundCfg struct {
	Filename      string `required:"true" description:"Socket filename, which will be overwritten"`
	Permissions   int    `description:"Socket file permissions" default:"0600"`
	RemoteNode    string `required:"true" description:"Receptor node to connect to"`
	RemoteService string `required:"true" description:"Receptor service name to connect to"`
	TLS           string `description:"Name of TLS client config for the Receptor connection"`
}

// Run runs the action.
func (cfg UnixProxyInboundCfg) Run() error {
	netceptor.MainInstance.Logger.Debug("Running Unix socket inbound proxy service %v\n", cfg)
	tlscfg, err := netceptor.MainInstance.GetClientTLSConfig(cfg.TLS, cfg.RemoteNode, netceptor.ExpectedHostnameTypeReceptor)
	if err != nil {
		return err
	}

	return UnixProxyServiceInbound(netceptor.MainInstance, cfg.Filename, os.FileMode(cfg.Permissions),
		cfg.RemoteNode, cfg.RemoteService, tlscfg)
}

// unixProxyOutboundCfg is the cmdline configuration object for a Unix socket outbound proxy.
type UnixProxyOutboundCfg struct {
	Service  string `required:"true" description:"Receptor service name to bind to"`
	Filename string `required:"true" description:"Socket filename, which must already exist"`
	TLS      string `description:"Name of TLS server config for the Receptor connection"`
}

// Run runs the action.
func (cfg UnixProxyOutboundCfg) Run() error {
	netceptor.MainInstance.Logger.Debug("Running Unix socket inbound proxy service %s\n", cfg)
	tlscfg, err := netceptor.MainInstance.GetServerTLSConfig(cfg.TLS)
	if err != nil {
		return err
	}

	return UnixProxyServiceOutbound(netceptor.MainInstance, cfg.Service, tlscfg, cfg.Filename)
}

func init() {
	version := viper.GetInt("version")
	if version > 1 {
		return
	}
	if runtime.GOOS != "windows" {
		cmdline.RegisterConfigTypeForApp("receptor-proxies",
			"unix-socket-server", "Listen on a Unix socket and forward via Receptor", UnixProxyInboundCfg{}, cmdline.Section(servicesSection))
		cmdline.RegisterConfigTypeForApp("receptor-proxies",
			"unix-socket-client", "Listen via Receptor and forward to a Unix socket", UnixProxyOutboundCfg{}, cmdline.Section(servicesSection))
	}
}
