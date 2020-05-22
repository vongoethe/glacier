package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gorilla/mux"
	asteriaEvent "github.com/mylxsw/asteria/event"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/container"
	"github.com/mylxsw/glacier"
	"github.com/mylxsw/glacier/cron"
	"github.com/mylxsw/glacier/event"
	"github.com/mylxsw/glacier/example/api"
	"github.com/mylxsw/glacier/example/config"
	"github.com/mylxsw/glacier/example/job"
	"github.com/mylxsw/glacier/example/service"
	"github.com/mylxsw/glacier/starter/application"
	"github.com/mylxsw/glacier/web"
	"github.com/urfave/cli"
	"github.com/urfave/cli/altsrc"
)

var Version = "1.0"
var GitCommit = "aabbccddeeffgghhiijjkk"

type CronEvent struct{}

func main() {
	//log.All().LogFormatter(formatter.NewJSONFormatter())

	log.DefaultDynamicModuleName(true)
	log.AddGlobalFilter(func(filter log.Filter) log.Filter {
		return func(f asteriaEvent.Event) {
			if strings.HasPrefix(f.Module, "github.com.mylxsw.glacier.cron") {
				return
			}

			filter(f)
		}
	})

	app := application.Create(fmt.Sprintf("%s (%s)", Version, GitCommit[:8]))

	app.AddFlags(altsrc.NewStringFlag(cli.StringFlag{
		Name:  "test",
		Value: "",
	}))

	app.WithHttpServer().TCPListenerAddr(":19945")

	app.WebAppInit(func(cc container.Container, webApp *glacier.WebApp, conf *web.Config) error {
		// 设置该选项之后，路由匹配时将会忽略最末尾的 /
		// 路由 /aaa/bbb  匹配 /aaa/bbb, /aaa/bbb/
		// 路由 /aaa/bbb/ 匹配 /aaa/bbb, /aaa/bbb/
		// 默认为 false，匹配规则如下
		// 路由 /aaa/bbb 只匹配 /aaa/bbb 不匹配 /aaa/bbb/
		// 路由 /aaa/bbb/ 只匹配 /aaa/bbb/ 不匹配 /aaa/bbb
		conf.IgnoreLastSlash = true

		return nil
	})

	app.WebAppExceptionHandler(func(ctx web.Context, err interface{}) web.Response {
		log.Errorf("stack: %s", debug.Stack())
		return nil
	})

	app.Provider(job.ServiceProvider{})
	app.Provider(api.ServiceProvider{})

	app.Service(&service.DemoService{})
	app.Service(&service.Demo2Service{})

	app.Cron(func(cr cron.Manager, cc container.Container) error {
		if err := cr.Add("hello", "@every 15s", func(manager event.Manager) {
			log.Infof("hello, example!")
			manager.Publish(CronEvent{})
		}); err != nil {
			return err
		}

		return nil
	})

	app.EventListener(func(listener event.Manager, cc container.Container) {
		listener.Listen(func(event CronEvent) {
			log.Debug("a new cron task executed")
		})
	})

	app.Singleton(func(c glacier.FlagContext) *config.Config {
		return &config.Config{
			DB:   "xxxxxx",
			Test: c.String("test"),
		}
	})

	app.Main(func(conf *config.Config, router *mux.Router) {
		log.Debugf("config: %s", conf.Serialize())
		for _, r := range web.GetAllRoutes(router) {
			log.Debugf("route: %s -> %s | %s | %s", r.Name, r.Methods, r.PathTemplate, r.PathRegexp)
		}
	})

	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
