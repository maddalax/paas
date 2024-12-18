package deployment

import (
	"context"
	"dockman/app"
	"dockman/app/ui"
	"dockman/pages/resource/resourceui"
	"github.com/maddalax/htmgo/extensions/websocket/ws"
	"github.com/maddalax/htmgo/framework/h"
)

func RunLog(ctx *h.RequestContext) *h.Page {
	id := ctx.QueryParam("id")
	resource, err := app.ResourceGet(ctx.ServiceLocator(), id)

	if err != nil {
		ctx.Redirect("/", 302)
		return h.EmptyPage()
	}

	app.OnceWithAliveContext(ctx, func(context context.Context) {
		_ = app.StreamResourceLogs(ctx.ServiceLocator(), context, resource, func(log *app.DockerLog) {
			ws.PushElementCtx(ctx, ui.DockerLogLine(log))
		})
	})

	return resourceui.Page(ctx, func(resource *app.Resource) *h.Element {
		return h.Div(
			h.Class("w-[calc(100vh - 300px)] h-full"),
			ui.LogBody(ui.LogBodyOptions{
				MaxLogs: 1000,
			}),
		)
	})
}
