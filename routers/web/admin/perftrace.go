package admin

import (
	"net/http"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/tailmsg"
	"code.gitea.io/gitea/services/context"
)

func PerfTrace(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.monitor")
	ctx.Data["PageIsAdminMonitorTrace"] = true
	ctx.Data["PageIsAdminMonitorPerfTrace"] = true
	log.Warn("Getting records...")
	ctx.Data["PerfTraceRecords"] = tailmsg.GetManager().GetTraceRecorder().GetRecords()
	log.Info("%s", tailmsg.GetManager().GetTraceRecorder().GetRecords())
	ctx.HTML(http.StatusOK, tplPerfTrace)
}
