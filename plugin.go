package jrpc2

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
)

type Option struct {
	Name string
	Default string
	description string
	Val string
}

func NewOption(name, description, defaultValue string) *Option {
	return &Option{
		Name: name,
		Default: defaultValue,
		description: description,
	}
}

func (o *Option) Description() string {
	if o.description != "" {
		return o.description
	}

	return "A golightning plugin option"
}

func (o *Option) Set(value string) {
	o.Val = value
}

func (o *Option) Value() string {
	return o.Val
}

func (o *Option) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name string		`json:"name"`
		Type string		`json:"type"`
		Default string		`json:"default,omitempty"`
		Description string	`json:"description"`
	}{
		Name: o.Name,
		Type: "string", // all options are type string atm
		Default: o.Default,
		Description: o.Description(),
	})
}

// we don't need to unmarshal this, since
// we don't expect to ever get a callback with this struct
// func (o *Option) UnmarshalJSON([]byte) error {}

type RpcMethod struct {
	Method ServerMethod
	Desc string
}

func NewRpcMethod(method ServerMethod, desc string) *RpcMethod {
	return &RpcMethod{
		Method: method,
		Desc: desc,
	}
}

func (r *RpcMethod) Description() string {
	if r.Desc != "" {
		return r.Desc
	}

	return "A golightning RPC method."
}

func (r *RpcMethod) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Name string		`json:"name"`
		Description string	`json:"description"`
		Params []string		`json:"params,omitempty"`
	}{
		Name: r.Method.Name(),
		Description: r.Description(),
		Params: getParamList(r.Method),
	})
}

type GetManifestMethod struct {
	plugin *Plugin
}

func (gm *GetManifestMethod) New() interface{} {
	method := &GetManifestMethod{}
	method.plugin = gm.plugin
	return method
}

func NewManifestRpcMethod(p *Plugin) *RpcMethod {
	return &RpcMethod{
		Method: &GetManifestMethod{
			plugin: p,
		},
		Desc: "Generate manifest for plugin",
	}
}

type Manifest struct {
	Options []*Option `json:"options"`
	RpcMethods []*RpcMethod `json:"rpcmethods"`
}

func (gm GetManifestMethod) Name() string {
	return "getmanifest"
}

// Don't include 'built-in' methods in manifest list
func isBuiltInMethod(name string) bool {
	return name == "getmanifest" ||
		name == "init"
}

// Builds the manifest object that's returned from the
// `getmanifest` method.
func (gm GetManifestMethod) Call() (Result, error) {
	m := &Manifest{}
	m.RpcMethods = make([]*RpcMethod, 0, len(gm.plugin.methods))
	for _, rpc := range gm.plugin.methods {
		if !isBuiltInMethod(rpc.Method.Name()) {
			m.RpcMethods = append(m.RpcMethods, rpc)
		}
	}

	m.Options = make([]*Option, len(gm.plugin.options))
	i := 0
	for _, option := range gm.plugin.options {
		m.Options[i] = option
		i++
	}

	return m, nil
}

type Config struct {
	LightningDir string	`json:"lightning-dir"`
	RpcFile string		`json:"rpc-file"`
}

type InitMethod struct {
	Options map[string]string	`json:"options"`
	Configuration *Config		`json:"configuration"`
	plugin *Plugin
}

func NewInitRpcMethod(p *Plugin) *RpcMethod {
	return &RpcMethod{
		Method: &InitMethod{
			plugin:  p,
		},
	}
}

func (im InitMethod) New() interface{} {
	method := &InitMethod{}
	method.plugin = im.plugin
	return method
}

func (im InitMethod) Name() string {
	return "init"
}

func (im InitMethod) Call() (Result, error) {
	// fill in options
	for name, value := range im.Options {
		option, exists := im.plugin.options[name]
		if !exists {
			log.Printf("No option %s registered on this plugin", name)
			continue
		}
		opt := option
		opt.Set(value)
	}
	// stash the config...
	im.plugin.Config = im.Configuration
	im.plugin.initialized = true

	// call init hook
	im.plugin.initFn(im.plugin, im.plugin.getOptionSet(), im.Configuration)

	// Result of `init` is currently discarded by c-light
	return "ok", nil
}

type Plugin struct {
	server *Server
	options map[string]*Option
	methods map[string]*RpcMethod
	initialized bool
	initFn func(plugin *Plugin, options map[string]string, c *Config)
	logFile string
	Config *Config
}

func (p *Plugin) SetLogfile(filename string) {
	p.logFile = filename
}

func NewPlugin(initHandler func(p *Plugin, o map[string]string, c *Config)) *Plugin {
	plugin := &Plugin{}
	plugin.server = NewServer()
	plugin.options = make(map[string]*Option)
	plugin.methods = make(map[string]*RpcMethod)
	plugin.initFn = initHandler
	plugin.logFile = "golightning.log"
	return plugin
}

func (p *Plugin) Start(in, out *os.File) error {
	logFile := checkForMonkeyPatch(out, p.logFile)
	if logFile != nil {
		defer logFile.Close()
	}
	// register the init & getmanifest commands
	p.RegisterMethod(NewManifestRpcMethod(p))
	p.RegisterMethod(NewInitRpcMethod(p))
	return p.server.StartUp(in, out)
}

// Remaps stdout to a logfile, if configured to run over stdout
func checkForMonkeyPatch(out *os.File, logfile string) *os.File {
	_, isLN := os.LookupEnv("LIGHTNINGD_PLUGIN")
	if out.Fd() != os.Stdout.Fd() || !isLN {
		return nil
	}

	// set up the logger to redirect out to a log file (for now).
	// todo: send the logs to the lightning-d channel instead
	f, err := os.OpenFile(logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		// we really don't want to start up if we can't write out 
		panic(err.Error())
	}
	// hehehe
	log.SetOutput(os.Stderr)
	return f
}


func (p *Plugin) RegisterMethod(m *RpcMethod) error {
	err := p.server.Register(m.Method)
	if err != nil {
		return err
	}
	err = p.registerRpcMethod(m)
	if err != nil {
		p.server.Unregister(m.Method)
	}
	return err
}

func (p *Plugin) registerRpcMethod(rpc *RpcMethod) error {
	if rpc == nil || rpc.Method == nil {
		return fmt.Errorf("Can't register an empty rpc method")
	}
	m := rpc.Method
	if _, exists := p.methods[m.Name()]; exists {
		return fmt.Errorf("Method `%s` already registered", m.Name())
	}
	p.methods[m.Name()] = rpc
	return nil
}


func (p *Plugin) UnregisterMethod(rpc *RpcMethod) error {
	// potentially munges the error code from server
	// but we don't really care as long as the method
	// is no longer registered either place.
	err := p.unregisterMethod(rpc)
	if err != nil || rpc.Method != nil {
		err = p.server.Unregister(rpc.Method)
	}
	return err
}

func (p *Plugin) unregisterMethod(rpc *RpcMethod) error {
	if rpc == nil || rpc.Method == nil {
		return fmt.Errorf("Can't unregister an empty method")
	}
	m := rpc.Method
	if _, exists := p.methods[m.Name()]; !exists {
		fmt.Errorf("Can't unregister, method %s is unknown", m.Name())
	}
	delete(p.methods, m.Name())
	return nil
}

func (p *Plugin) RegisterOption(o *Option) error {
	if o == nil {
		return fmt.Errorf("Can't register an empty option")
	}
	if _, exists := p.options[o.Name]; exists {
		return fmt.Errorf("Option `%s` already registered", o.Name)
	}
	p.options[o.Name] = o
	return nil
}

func (p *Plugin) UnregisterOption(o *Option) error {
	if o == nil {
		return fmt.Errorf("Can't remove an empty option")
	}
	if _, exists := p.options[o.Name]; !exists {
		return fmt.Errorf("No %s option registered", o.Name)
	}
	delete(p.options, o.Name)
	return nil
}

func (p *Plugin) GetOption(name string) *Option {
	return p.options[name]
}

func (p *Plugin) getOptionSet() map[string]string {
	options := make(map[string]string, len(p.options))
	for key, option := range p.options {
		options[key] = option.Value()
	}
	return options
}

func getParamList(method ServerMethod) []string {
	paramList := make([]string, 0)
	v := reflect.Indirect(reflect.ValueOf(method))

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.CanInterface() {
			continue
		}
		paramList = append(paramList, field.Type().Name())
	}
	return paramList
}