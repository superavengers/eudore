package main

/*
注册的扩展转换函数实现Context处理对象转换，通过转换后对象实现扩展。
*/

import (
	"github.com/eudore/eudore"
)

// MyContext 定义新的请求上下文扩展。
type MyContext struct {
	eudore.Context
}

// Hello 方法返回hello。
func (ctx MyContext) Hello() {
	ctx.WriteString("hello")
}

func main() {
	eudore.RegisterHandlerFunc(func(fn func(MyContext)) eudore.HandlerFunc {
		return func(ctx eudore.Context) {
			fn(MyContext{ctx})
		}
	})

	app := eudore.NewCore()
	app.GetFunc("/*", func(ctx MyContext) {
		ctx.Hello()
	})
	app.Listen(":8088")
	app.Run()
}