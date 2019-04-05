package main

import (
        "fmt"
        "io/ioutil"
//      "encoding/json"
//      "bytes"
        "context"
        "sync"
        "net/http"

        "github.com/inconshreveable/log15"

        "github.com/CyCoreSystems/ari"
        "github.com/CyCoreSystems/ari/client/native"
        "github.com/CyCoreSystems/ari/ext/play"
        "github.com/CyCoreSystems/ari/rid"
        "github.com/pkg/errors"
)

var ariApp = "test"

var log = log15.New()

var bridge *ari.BridgeHandle

func main() {

        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()

        // connect
        log.Info("Connecting to ARI")
        cl, err := native.Connect(&native.Options{
                Application:  "hello-world",
                Username:     "asterisk",
                Password:     "asterisk",
                URL:          "http://localhost:8088/ari",
                WebsocketURL: "ws://localhost:8088/ari/events",
        })
        if err != nil {
                log.Error("Failed to build ARI client", "error", err)
                return
        }

        // setup app 

        log.Info("Starting listener app")

        log.Info("Listening for new calls")
        sub := cl.Bus().Subscribe(nil, "StasisStart")

        for {
                select {
                case e := <-sub.Events():
                        v := e.(*ari.StasisStart)
                        log.Info("Got stasis start", "channel", v.Channel.ID)
                        go app(ctx, cl, cl.Channel().Get(v.Key(ari.ChannelKey, v.Channel.ID)))
                case <-ctx.Done():
                        return
                }
        }
}

func app(ctx context.Context, cl ari.Client, h *ari.ChannelHandle) {
        log.Info("running app", "channel", h.Key().ID)

        if err := h.Answer(); err != nil {
                log.Error("failed to answer call", "error", err)
                //return
        }

        //dialing to user
        if err := h.Dial("1554355381.25969", 1); err != nil {
                log.Error("failed to Dial call", "error", err)
                //return
        }

        //hiting curl
        // response, err := http.Get("http://localhost:8088/ari/bridges?api_key=asterisk:asterisk")
        // if err != nil {
        //         fmt.Printf("The HTTP request failed with error %s\n", err)
        // } else {
        //         data, _ := ioutil.ReadAll(response.Body)
        //         fmt.Println(string(data))
        // }


        if err := ensureBridge(ctx, cl, h.Key()); err != nil {
                log.Error("failed to manage bridge", "error", err)
                return
        }

        if err := bridge.AddChannel(h.Key().ID); err != nil {
                log.Error("failed to add channel to bridge", "error", err)
                return
        }

        log.Info("channel added to bridge")
        return
}

type bridgeManager struct {
        h *ari.BridgeHandle
}

func ensureBridge(ctx context.Context, cl ari.Client, src *ari.Key) (err error) {
        if bridge != nil {
                log.Debug("Bridge already exists")
                return nil
        }

        key := src.New(ari.BridgeKey, rid.New(rid.Bridge))
        bridge, err = cl.Bridge().Create(key, "mixing", key.ID)
        if err != nil {
                bridge = nil
                return errors.Wrap(err, "failed to create bridge")
        }

        wg := new(sync.WaitGroup)
        wg.Add(1)
        go manageBridge(ctx, bridge, wg)
        wg.Wait()

        return nil
}

func manageBridge(ctx context.Context, h *ari.BridgeHandle, wg *sync.WaitGroup) {
        // Delete the bridge when we exit
        defer h.Delete()

        destroySub := h.Subscribe(ari.Events.BridgeDestroyed)
        defer destroySub.Cancel()

        enterSub := h.Subscribe(ari.Events.ChannelEnteredBridge)
        defer enterSub.Cancel()

        leaveSub := h.Subscribe(ari.Events.ChannelLeftBridge)
        defer leaveSub.Cancel()

        wg.Done()
        for {
                select {
                case <-ctx.Done():
                        return
                case <-destroySub.Events():
                        log.Debug("bridge destroyed")
                        return
                case e, ok := <-enterSub.Events():
                        if !ok {
                                log.Error("channel entered subscription closed")
                                return
                        }
                        v := e.(*ari.ChannelEnteredBridge)
                        log.Debug("channel entered bridge", "channel", v.Channel.Name)
                        go func() {
                                if err := play.Play(ctx, h, play.URI("sound:confbridge-join")).Err(); err != nil {
                                        log.Error("failed to play join sound", "error", err)
                                }
                        }()
                case e, ok := <-leaveSub.Events():
                        if !ok {
                                log.Error("channel left subscription closed")
                                return
                        }
                        v := e.(*ari.ChannelLeftBridge)
                        log.Debug("channel left bridge", "channel", v.Channel.Name)
                        go func() {
                                if err := play.Play(ctx, h, play.URI("sound:confbridge-leave")).Err(); err != nil {
                                        log.Error("failed to play leave sound", "error", err)
                                }
                        }()
                }
        }
}


type OriginateRequest struct {

    // Endpoint is the name of the Asterisk resource to be used to create the
    // channel.  The format is tech/resource.
    //
    // Examples:
    //
    //   - PJSIP/george
    //
    //   - Local/party@mycontext
    //
    //   - DAHDI/8005558282
    Endpoint string `json:"endpoint"`

    // Timeout specifies the number of seconds to wait for the channel to be
    // answered before giving up.  Note that this is REQUIRED and the default is
    // to timeout immediately.  Use a negative value to specify no timeout, but
    // be aware that this could result in an unlimited call, which could result
    // in a very unfriendly bill.
    Timeout int `json:"timeout,omitempty"`

    // CallerID specifies the Caller ID (name and number) to be set on the
    // newly-created channel.  This is optional but recommended.  The format is
    // `"Name" <number>`, but most every component is optional.
    //
    // Examples:
    //
    //   - "Jane" <100>
    //
    //   - <102>
    //
    //   - 8005558282
    //
    CallerID string `json:"callerId,omitempty"`

    // CEP (Context/Extension/Priority) is the location in the Asterisk dialplan
    // into which the newly created channel should be dropped.  All of these are
    // required if the CEP is used.  Exactly one of CEP or App/AppArgs must be
    // specified.
    Context   string `json:"context,omitempty"`
    Extension string `json:"extension,omitempty"`
    Priority  int64  `json:"priority,omitempty"`

    // The Label is the string form of Priority, if there is such a label in the
    // dialplan.  Like CEP, Label may not be used if an ARI App is specified.
    // If both Label and Priority are specified, Label will take priority.
    Label string `json:"label,omitempty"`

    // App specifies the ARI application and its arguments into which
    // the newly-created channel should be placed.  Exactly one of CEP or
    // App/AppArgs is required.
    App string `json:"app,omitempty"`

    // AppArgs defines the arguments to supply to the ARI application, if one is
    // defined.  It is optional but only applicable for Originations which
    // specify an ARI App.
    AppArgs string `json:"appArgs,omitempty"`

    // Formats describes the (comma-delimited) set of codecs which should be
    // allowed for the created channel.  This is an optional parameter, and if
    // an Originator is specified, this should be left blank so that Asterisk
    // derives the codecs from that Originator channel instead.
    //
    // Ex. "ulaw,slin16".
    //
    // The list of valid codecs can be found with Asterisk command "core show codecs".
    Formats string `json:"formats,omitempty"`

    // ChannelID specifies the unique ID to be used for the channel to be
    // created.  It is optional, and if not specified, a time-based UUID will be
    // generated.
    ChannelID string `json:"channelId,omitempty"` // Optionally assign channel id

    // OtherChannelID specifies the unique ID of the second channel to be
    // created.  This is only valid for the creation of Local channels, which
    // are always generated in pairs.  It is optional, and if not specified, a
    // time-based UUID will be generated (again, only if the Origination is of a
    // Local channel).
    OtherChannelID string `json:"otherChannelId,omitempty"`

    // Originator is the channel for whom this Originate request is being made, if there is one.
    // It is used by Asterisk to set the right codecs (and possibly other parameters) such that
    // when the new channel is bridged to the Originator channel, there should be no transcoding.
    // This is a purely optional (but helpful, where applicable) field.
    Originator string `json:"originator,omitempty"`

    // Variables describes the set of channel variables to apply to the new channel.  It is optional.
    Variables map[string]string `json:"variables,omitempty"`
}


type Key struct {
    // Kind indicates the type of resource the Key points to.  e.g., "channel",
    // "bridge", etc.
    Kind string `protobuf:"bytes,1,opt,name=kind,proto3" json:"kind,omitempty"`
    // ID indicates the unique identifier of the resource
    ID  string `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
    // Node indicates the unique identifier of the Asterisk node on which the
    // resource exists or will be created
    Node string `protobuf:"bytes,3,opt,name=node,proto3" json:"node,omitempty"`
    // Dialog indicates a named scope of the resource, for receiving events
    Dialog string `protobuf:"bytes,4,opt,name=dialog,proto3" json:"dialog,omitempty"`
    // App indiciates the ARI application that this key is bound to.
    App string `protobuf:"bytes,5,opt,name=app,proto3" json:"app,omitempty"`
}
var key Key
key.Kind = "channel"
key.ID = "111112"


var or OriginateRequest

or.Endpoint = "SIP/6002"
or.Timeout = 1
or.CallerID = "Prabhat"
or.Context = "aritest"
or.Extension = "8888"
or.App = "hello-world"
or.ChannelID = "111111"


Originate(*key, or) (*ChannelHandle, error)
