package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/apigear-io/cli/pkg/helper"
	"github.com/apigear-io/cli/pkg/log"
	"github.com/apigear-io/objectlink-core-go/olink/client"
	"github.com/apigear-io/objectlink-core-go/olink/core"
	"github.com/apigear-io/objectlink-core-go/olink/ws"
	"github.com/spf13/cobra"
)

// client messages supported for feed
// - ["link", "demo.Calc"]
// - ["set", "demo.Calc/total", 20]
// - ["invoke", 1, "demo.Calc/add", [1]]
// - ["unlink", "demo.Calc"]
// server messages not supported for feed
// - ["init", "demo.Calc", { "total": 10 }]
// - ["change", "demo.Calc/total", 20]
// - ["reply", 1, "demo.Calc/add", 21]
// - ["signal", "demo.Calc/clearDone", []]
// - ["error", "init", 0, "init error"]

type ObjectSink struct {
	objectId string
}

func (s *ObjectSink) ObjectId() string {
	return s.objectId
}

func (s *ObjectSink) HandleSignal(signalId string, args core.Args) {
	log.Info().Msgf("<- signal %s(%v)", signalId, args)
}
func (s *ObjectSink) HandlePropertyChange(propertyId string, value core.Any) {
	log.Info().Msgf("<- property %s = %v", propertyId, value)
}
func (s *ObjectSink) HandleInit(objectId string, props core.KWArgs, node *client.Node) {
	s.objectId = objectId
	log.Info().Msgf("<- init %s with %v", objectId, props)
}
func (s *ObjectSink) HandleRelease() {
	log.Info().Msgf("<- release %s", s.objectId)
	s.objectId = ""
}

var _ client.IObjectSink = &ObjectSink{}

func NewClientCommand() *cobra.Command {
	type ClientOptions struct {
		addr   string
		script string
		sleep  time.Duration
		repeat int
	}
	var options = &ClientOptions{}
	// cmd represents the simCli command
	var cmd = &cobra.Command{
		Use:     "feed",
		Aliases: []string{"f"},
		Short:   "Feed simulation from command line",
		Long:    `Feed simulation calls using JSON documents from command line`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.script = args[0]
			log.Info().Str("script", options.script).Str("addr", options.addr).Int("repeat", options.repeat).Dur("sleep", options.sleep).Msg("feed simulation")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			registry := client.NewRegistry()
			registry.SetSinkFactory(func(objectId string) client.IObjectSink {
				return &ObjectSink{objectId: objectId}
			})
			log.Debug().Msgf("run script %s", options.script)
			conn, err := ws.Dial(ctx, options.addr)
			if err != nil {
				return err
			}
			defer func() {
				if err := conn.Close(); err != nil {
					log.Error().Err(err).Msg("failed to close connection")
				}
			}()
			node := client.NewNode(registry)
			conn.SetOutput(node)
			node.SetOutput(conn)
			registry.AttachClientNode(node)
			switch filepath.Ext(options.script) {
			case ".ndjson":
				items, err := helper.ScanFile(options.script)
				if err != nil {
					return err
				}
				ctrl := helper.NewSenderControl[[]byte](options.repeat, options.sleep)
				err = ctrl.Run(items, func(data []byte) error {
					log.Debug().Msgf("send -> %s", data)
					err := handleNodeData(node, data)
					if err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					log.Warn().Err(err).Msg("send error")
				}
			}
			<-ctx.Done()
			log.Info().Msg("done")
			return nil
		},
	}
	cmd.Flags().DurationVarP(&options.sleep, "sleep", "", 100, "sleep duration between messages")
	cmd.Flags().StringVarP(&options.addr, "addr", "", "ws://127.0.0.1:4333/ws", "address of the simulation server")
	cmd.Flags().IntVarP(&options.repeat, "repeat", "", 1, "number of times to repeat the script")
	return cmd
}

func handleNodeData(node *client.Node, data []byte) error {
	var m core.Message
	err := json.Unmarshal(data, &m)
	if err != nil {
		log.Error().Err(err).Msgf("invalid message: %s", data)
		return err
	}
	s, ok := m[0].(string)
	if !ok {
		log.Error().Msgf("invalid message type, expected string: %v", m)
		return fmt.Errorf("invalid message type, expected string: %v", m)
	}
	m[0] = core.MsgTypeFromString(s)
	switch m[0] {
	case core.MsgLink:
		objectId := m.AsLink()
		node.LinkRemoteNode(objectId)
	case core.MsgUnlink:
		objectId := m.AsLink()
		node.UnlinkRemoteNode(objectId)
	case core.MsgSetProperty:
		propertyId, value := m.AsSetProperty()
		node.SetRemoteProperty(propertyId, value)
	case core.MsgInvoke:
		_, methodId, args := m.AsInvoke()
		node.InvokeRemote(methodId, args, func(arg client.InvokeReplyArg) {
			log.Info().Msgf("<- reply %s : %v", arg.Identifier, arg.Value)
		})
	default:
		log.Info().Msgf("not supported message type: %v", m)
		return fmt.Errorf("not supported message type: %v", m)
	}
	return nil
}
