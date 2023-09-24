package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

type tunnelConf struct {
	network    string
	localAddr  string
	remoteAddr string
}

type tunnel struct {
	upstream net.Conn
	conf     tunnelConf
}

const (
	incomingConnChSize = 5
)

var lgr = zerolog.New(zerolog.ConsoleWriter{
	Out:          os.Stderr,
	TimeFormat:   "Jan _2 15:04:05.999",
	PartsExclude: []string{zerolog.CallerFieldName},
})

func main() {
	ctx := context.Background()

	var incomingConn = make(chan tunnel, incomingConnChSize)
	go processIncomingConnections(ctx, incomingConn)

	// Load tunnel configurations
	tunnelConfigs := loadConfig()
	if len(tunnelConfigs) == 0 {
		lgr.Fatal().Msg("no tunnel configurations are set")
	}
	eg, _ := errgroup.WithContext(ctx)
	for _, conf := range tunnelConfigs {
		conf := conf
		eg.Go(func() error {
			return listen(ctx, conf, incomingConn)
		})
	}
	if err := eg.Wait(); err != nil {
		lgr.Fatal().Err(err).Msg("failed to listen:")
	}
}

// loadConfig loads tunnel configurations.
func loadConfig() []tunnelConf {
	var tunnelConfigs []tunnelConf
	// Iterate over all TUNNEL_* env vars until no more vars are found.
	for i := 1; true; i++ {
		val, ok := os.LookupEnv(fmt.Sprintf("TUNNEL_%d", i))
		if !ok {
			break
		}
		conf, err := parseConfigString(val)
		if err != nil {
			lgr.Err(err).Msg("failed to parse tunnel config")
			continue
		}
		tunnelConfigs = append(tunnelConfigs, conf)
	}
	return tunnelConfigs
}

// parseConfigString parses a tunnel configuration string into tunnelConf.
// Expected format: LOCAL_ADDR>REMOTE_ADDR
//
// Example:
//
//	0.0.0.0:9090>postgres:5432
//	:9090>postgres:5432
func parseConfigString(str string) (tunnelConf, error) {
	split := strings.Split(str, ">")
	if len(split) != 2 {
		return tunnelConf{}, fmt.Errorf("invalid tunnel config: %s (expected format: 0.0.0.0:9090>postgres:5432)", str)
	}
	return tunnelConf{
		network:    "tcp",
		localAddr:  split[0],
		remoteAddr: split[1],
	}, nil
}

// processIncomingConnections processes incoming connections from the provided channel.
func processIncomingConnections(ctx context.Context, connCh chan tunnel) {
	for {
		select {
		case <-ctx.Done():
			return
		case tun := <-connCh:
			go func() {
				if err := runTunnel(ctx, tun); err != nil {
					lgr.Err(err).Msg("tunnel failed")
				}
			}()
		}
	}
}

// listen starts listening a single entry point incoming connections based provided configuration.
func listen(ctx context.Context, conf tunnelConf, connCh chan<- tunnel) error {
	// Start local listener
	ls, err := net.Listen(conf.network, conf.localAddr)
	if err != nil {
		return fmt.Errorf("failed to open local listener: %w", err)
	}
	lgr.Info().Str("addr", ls.Addr().String()).Msg("Listening for incoming connections")
	defer func() {
		_ = ls.Close()
		lgr.Info().Str("addr", ls.Addr().String()).Msg("Stopped listening for incoming connections")
	}()

	// Wait for ctx to be done
	go func() {
		<-ctx.Done()
		_ = ls.Close()
	}()

	// Start listener for incoming connections
	for {
		upstream, err := ls.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept incoming connection: %w", err)
		}
		lgr.Info().Str("addr", upstream.RemoteAddr().String()).Msg("Accepted incoming connection")

		// Put incoming connection to the processing channel
		connCh <- tunnel{
			upstream: upstream,
			conf:     conf,
		}
	}
}

// runTunnel opens a tunnel between upstream and downstream connections.
func runTunnel(ctx context.Context, tun tunnel) error {
	upstream := tun.upstream
	// Setup downstream connection
	downstream, err := net.Dial(tun.conf.network, tun.conf.remoteAddr)
	if err != nil {
		_ = upstream.Close()
		return fmt.Errorf("failed to open downstream connection: %w", err)
	}
	// Close connections on exit
	defer func() {
		_ = upstream.Close()
		_ = downstream.Close()
	}()

	// Open the tunnel
	eg, _ := errgroup.WithContext(ctx)
	eg.Go(func() error {
		_, err := io.Copy(downstream, upstream)
		return err
	})
	eg.Go(func() error {
		_, err := io.Copy(upstream, downstream)
		return err
	})
	lgr.Info().Msgf("Tunnel is open between %s and %s", upstream.RemoteAddr(), tun.conf.remoteAddr)
	defer lgr.Info().Msgf("Tunnel is open between %s and %s", upstream.RemoteAddr(), tun.conf.remoteAddr)
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("tunnel error occurred: %w", err)
	}
	return nil
}
