package eudore

/*
保存各种全局函数，用于根据名称获得对应的函数。
*/

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	// etcd "github.com/coreos/etcd/client"
)

// 保存全局函数
var (
	GlobalRouterCheckFunc    = make(map[string]RouterCheckFunc)
	GlobalRouterNewCheckFunc = make(map[string]RouterNewCheckFunc)
	GlobalConfigReadFunc     = make(map[string]ConfigReadFunc)
)

func init() {
	// RouterCheckFunc
	GlobalRouterCheckFunc["isnum"] = RouterCheckFuncIsnum
	// RouterNewCheckFunc
	GlobalRouterNewCheckFunc["min"] = RouterNewCheckFuncMin
	GlobalRouterNewCheckFunc["regexp"] = RouterNewCheckFuncRegexp
	// ConfigReadFunc
	GlobalConfigReadFunc["default"] = ConfigReadFile
	GlobalConfigReadFunc["file"] = ConfigReadFile
	GlobalConfigReadFunc["https"] = ConfigReadHTTP
	GlobalConfigReadFunc["http"] = ConfigReadHTTP
}

// ConfigParseRead 函数使用'keys.config'读取配置内容，并使用[]byte类型保存到'keys.configdata'。
func ConfigParseRead(c Config) error {
	errs := NewErrors()
	for _, path := range GetArrayString(c.Get("keys.config")) {
		// read protocol and get read func
		s := strings.SplitN(path, "://", 2)
		fn := GlobalConfigReadFunc[s[0]]
		if fn == nil {
			// use default read func
			fn = GlobalConfigReadFunc["default"]
		}
		data, err := fn(path)
		if err == nil {
			c.Set("keys.configdata", data)
			c.Set("keys.configpath", path)
			return nil
		}
		errs.HandleError(err)
	}
	return errs.GetError()
}

// ConfigParseConfig 函数获得'keys.configdata'的内容解析配置。
func ConfigParseConfig(c Config) error {
	data := c.Get("keys.configdata")
	if data == nil {
		return nil
	}
	err := json.Unmarshal(data.([]byte), c)
	return err
}

// ConfigParseArgs 函数使用参数设置配置，参数使用--为前缀。
func ConfigParseArgs(c Config) (err error) {
	for _, str := range os.Args[1:] {
		if !strings.HasPrefix(str, "--") {
			continue
		}
		c.Set(split2byte(str[2:], '='))
	}
	return
}

// ConfigParseEnvs 函数使用环境变量设置配置，环境变量使用'ENV_'为前缀,'_'下划线相当于'.'的作用。
func ConfigParseEnvs(c Config) error {
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "ENV_") {
			k, v := split2byte(value, '=')
			k = strings.ToLower(strings.Replace(k, "_", ".", -1))[4:]
			c.Set(k, v)
		}
	}
	return nil
}

// ConfigParseMods 函数从'enable'项获得使用的模式的数组字符串，从'mods.xxx'加载配置。
//
// 默认会加载OS mod,如果是docker环境下使用docker模式。
func ConfigParseMods(c Config) error {
	mod, ok := c.Get("enable").([]string)
	if !ok {
		modi, ok := c.Get("enable").([]interface{})
		if ok {
			mod = make([]string, len(modi))
			for i, s := range modi {
				mod[i] = fmt.Sprint(s)
			}
		} else {
			return nil
		}
	}
	mod = append(mod, getOS())
	for _, i := range mod {
		ConvertTo(c.Get("mods."+i), c.Get(""))
	}
	return nil
}

func getOS() string {
	// check docker
	_, err := os.Stat("/.dockerenv")
	if err == nil || !os.IsNotExist(err) {
		return "docker"
	}
	// 返回默认OS
	return runtime.GOOS
}

// ConfigParseHelp 函数测试配置内容，如果存在'keys.help'项会使用JSON标准化输出配置到标准输出。
func ConfigParseHelp(c Config) error {
	ok := c.Get("keys.help") != nil
	if ok {
		JSON(c)
	}
	return nil
}

// ConfigReadFile Read config file
func ConfigReadFile(path string) ([]byte, error) {
	if strings.HasPrefix(path, "file://") {
		path = path[7:]
	}
	data, err := ioutil.ReadFile(path)
	last := strings.LastIndex(path, ".") + 1
	if last == 0 {
		return nil, fmt.Errorf("read file config, type is null")
	}
	return data, err
}

// ConfigReadHTTP Send http request get config info
func ConfigReadHTTP(path string) ([]byte, error) {
	resp, err := http.Get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	return data, err
}

// example: etcd://127.0.0.1:2379/config
/*func ConfigReadEtcd(path string) (string, error) {
	server, key := split2byte(path[7:], '/')
	cfg := etcd.Config{
		Endpoints:               []string{"http://" + server},
		Transport:               etcd.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
	c, err := etcd.New(cfg)
	if err != nil {
		return "", err
	}
	kapi := etcd.NewKeysAPI(c)
	resp, err := kapi.Get(context.Background(), key, nil)
	return resp.Node.Value, err
}*/

// InitSignal 函数定义初始化系统信号。
func InitSignal(app *Eudore) error {
	if runtime.GOOS == "windows" || GetStringBool(os.Getenv(EnvEudoreDisableSignal)) {
		return nil
	}

	// Register signal
	app.RegisterSignal(syscall.Signal(0x2), func(app *Eudore) error {
		app.WithField("signal", 2).Info("eudore received SIGINT, eudore shutting down HTTP server.")
		return app.Shutdown()
	})
	app.RegisterSignal(syscall.Signal(0xc), func(app *Eudore) error {
		app.WithField("signal", 12).Info("eudore received SIGUSR2, eudore restarting HTTP server.")
		return app.Restart()
	})
	app.RegisterSignal(syscall.Signal(0xf), func(app *Eudore) error {
		app.WithField("signal", 15).Info("eudore received SIGTERM, eudore shutting down HTTP server.")
		return app.Shutdown()
	})

	return nil
}

// InitConfig 函数定义解析配置。
func InitConfig(app *Eudore) error {
	return app.Config.Parse()
}

// InitWorkdir 函数初始化工作空间，从config获取workdir的值为工作空间，然后切换目录。
func InitWorkdir(app *Eudore) error {
	dir := GetString(app.Config.Get("workdir"))
	if dir != "" {
		app.Logger.Info("changes working directory to: " + dir)
		return os.Chdir(dir)
	}
	return nil
}

// InitLoggerStd 初始化日志组件。
func InitLoggerStd(app *Eudore) error {
	initlog, ok := app.Logger.(LoggerInitHandler)
	if !ok {
		return nil
	}

	// 创建LoggerStd
	key := GetDefaultString(app.Config.Get("keys.logger"), "component.logger")
	log, err := NewLoggerStd(app.Config.Get(key))
	if err != nil {
		return err
	}

	// 设置Logger
	app.Logger = log
	initlog.NextHandler(app.Logger)
	Set(app.Router, "print", NewLoggerPrintFunc(app.Logger))
	Set(app.Server, "print", NewLoggerPrintFunc(app.Logger))
	return nil
}

// InitStart 函数启动Eudore Server。
func InitStart(app *Eudore) error {
	// 更新context func，设置server处理者。
	if fn, ok := app.Config.Get("keys.context").(PoolGetFunc); ok {
		app.ContextPool.New = fn
	}
	if h, ok := app.Config.Get("keys.handler").(http.Handler); ok {
		app.Server.SetHandler(h)
	} else {
		app.Server.SetHandler(app)
	}

	// 监听全部配置
	lns, err := newServerListens(app.Config.Get("listeners"))
	if err != nil {
		return err
	}
	for i := range lns {
		ln, err := lns[i].Listen()
		if err != nil {
			app.Error(err)
			continue
		}
		app.AddListener(ln)
	}
	return nil
}

// RouterCheckFuncIsnum 检查字符串是否为数字。
func RouterCheckFuncIsnum(arg string) bool {
	_, err := strconv.Atoi(arg)
	return err == nil
}

// RouterNewCheckFuncMin 生成一个检查字符串最小值的RouterCheckFunc函数。
func RouterNewCheckFuncMin(str string) RouterCheckFunc {
	n, err := strconv.Atoi(str)
	if err != nil {
		return nil
	}
	return func(arg string) bool {
		num, err := strconv.Atoi(arg)
		if err != nil {
			return false
		}
		return num >= n
	}
}

// RouterNewCheckFuncRegexp 生成一个正则匹配的RouterCheckFunc函数。
func RouterNewCheckFuncRegexp(str string) RouterCheckFunc {
	// 创建正则表达式
	re, err := regexp.Compile(str)
	if err != nil {
		return nil
	}
	// 返回正则匹配校验函数
	return func(arg string) bool {
		return re.MatchString(arg)
	}
}
