package cmd

import (
	"context"
	"log"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/pojntfx/webrtcfd/pkg/wrtcconn"
	"github.com/pojntfx/webrtcfd/pkg/wrtceth"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	devFlag      = "dev"
	macFlag      = "mac"
	parallelFlag = "parallel"
)

var ethernetCmd = &cobra.Command{
	Use:     "ethernet",
	Aliases: []string{"eth", "e"},
	Short:   "Join a layer 2 overlay network",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if strings.TrimSpace(viper.GetString(communityFlag)) == "" {
			return errMissingCommunity
		}

		if strings.TrimSpace(viper.GetString(passwordFlag)) == "" {
			return errMissingPassword
		}

		if strings.TrimSpace(viper.GetString(keyFlag)) == "" {
			return errMissingKey
		}

		u, err := url.Parse(viper.GetString(raddrFlag))
		if err != nil {
			return err
		}

		q := u.Query()
		q.Set("community", viper.GetString(communityFlag))
		q.Set("password", viper.GetString(passwordFlag))
		u.RawQuery = q.Encode()

		adapter := wrtceth.NewAdapter(
			u.String(),
			viper.GetString(keyFlag),
			viper.GetStringSlice(iceFlag),
			&wrtceth.AdapterConfig{
				Device: viper.GetString(devFlag),
				OnSignalerConnect: func(s string) {
					log.Println("Connected to signaler as", s)
				},
				OnPeerConnect: func(s string) {
					log.Println("Connected to peer", s)
				},
				OnPeerDisconnected: func(s string) {
					log.Println("Disconnected from peer", s)
				},
				Parallel: viper.GetInt(parallelFlag),
				AdapterConfig: &wrtcconn.AdapterConfig{
					Timeout:    viper.GetDuration(timeoutFlag),
					Verbose:    viper.GetBool(verboseFlag),
					ID:         viper.GetString(macFlag),
					ForceRelay: viper.GetBool(forceRelayFlag),
				},
			},
			ctx,
		)

		log.Println("Connecting to signaler", viper.GetString(raddrFlag))

		if err := adapter.Open(); err != nil {
			return err
		}
		defer func() {
			if err := adapter.Close(); err != nil {
				panic(err)
			}
		}()

		return adapter.Wait()
	},
}

func init() {
	ethernetCmd.PersistentFlags().String(raddrFlag, "wss://webrtcfd.herokuapp.com/", "Remote address")
	ethernetCmd.PersistentFlags().Duration(timeoutFlag, time.Second*10, "Time to wait for connections")
	ethernetCmd.PersistentFlags().String(communityFlag, "", "ID of community to join")
	ethernetCmd.PersistentFlags().String(passwordFlag, "", "Password for community")
	ethernetCmd.PersistentFlags().String(keyFlag, "", "Encryption key for community")
	ethernetCmd.PersistentFlags().StringSlice(iceFlag, []string{"stun:stun.l.google.com:19302"}, "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	ethernetCmd.PersistentFlags().Bool(forceRelayFlag, false, "Force usage of TURN servers")
	ethernetCmd.PersistentFlags().String(devFlag, "", "Name to give to the TAP device (i.e. weron0) (default is auto-generated; only supported on Linux, macOS and Windows)")
	ethernetCmd.PersistentFlags().String(macFlag, "", "MAC address to give to the TAP device (i.e. 3a:f8:de:7b:ef:52) (default is auto-generated; only supported on Linux)")
	ethernetCmd.PersistentFlags().Int(parallelFlag, runtime.NumCPU(), "Amount of threads to use to decode frames")

	viper.AutomaticEnv()

	rootCmd.AddCommand(ethernetCmd)
}