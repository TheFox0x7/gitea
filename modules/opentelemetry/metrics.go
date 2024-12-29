package opentelemetry

import (
	"context"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

const namespace = "gitea_"

func setupMetricProvider(ctx context.Context, r *resource.Resource) (func(context.Context) error, error) {
	var shutdown func(context.Context) error
	shutdown = func(ctx context.Context) error { return nil }
	if setting.Metrics.Enabled {
		metricExporter, err := prometheus.New(prometheus.WithNamespace(namespace))
		if err != nil {
			return nil, err
		}
		metricProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricExporter), sdkmetric.WithResource(r))
		otel.SetMeterProvider(metricProvider)
		shutdown = metricExporter.Shutdown
	}
	defineCurrentMetrics()
	return shutdown, nil
}

func defineCurrentMetrics() error {
	meter := otel.Meter("gitea")
	if _, err := meter.Int64ObservableGauge("accesses", metric.WithDescription("Number of Accesses"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Access)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("attachments", metric.WithDescription("Number of Attachments"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Attachment)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("comments", metric.WithDescription("Number of Comments"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Comment)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("follows", metric.WithDescription("Number of Follows"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Follow)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("hooktasks", metric.WithDescription("Number of HookTasks"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.HookTask)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("issues", metric.WithDescription("Number of Issues"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Issue)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("labels", metric.WithDescription("Number of Labels"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Label)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("loginsources", metric.WithDescription("Number of LoginSources"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.AuthSource)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("milestones", metric.WithDescription("Number of Milestones"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Milestone)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("mirrors", metric.WithDescription("Number of Mirrors"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Mirror)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("oauths", metric.WithDescription("Number of Oauths"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Oauth)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("organizations", metric.WithDescription("Number of Organizations"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Org)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("projects", metric.WithDescription("Number of Projects"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Project)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("projects_boards", metric.WithDescription("Number of project columns"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.ProjectColumn)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("publickeys", metric.WithDescription("Number of PublicKeys"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.PublicKey)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("releases", metric.WithDescription("Number of Releases"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Release)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("repositories", metric.WithDescription("Number of Repositories"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Repo)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("teams", metric.WithDescription("Number of Teams"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Team)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("updatetasks", metric.WithDescription("Number of UpdateTasks"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.UpdateTask)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("watches", metric.WithDescription("Number of Watches"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Oauth)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	if _, err := meter.Int64ObservableGauge("webhooks", metric.WithDescription("Number of Webhooks"), metric.WithInt64Callback(
		func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(activities_model.GetStatistic(ctx).Counter.Webhook)
			return nil
		}),
	); err != nil {
		log.Warn("Setting metric failed: %s", err)
	}
	return nil
}
