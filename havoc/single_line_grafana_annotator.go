package havoc

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/grafana"
)

// SingleLineGrafanaAnnotator annotates Grafana dashboards with chaos experiment events in a single line.
type SingleLineGrafanaAnnotator struct {
	client       *grafana.Client
	dashboardUID string
	logger       zerolog.Logger
}

func NewSingleLineGrafanaAnnotator(grafanaURL, grafanaToken, dashboardUID string, logger zerolog.Logger) *SingleLineGrafanaAnnotator {
	return &SingleLineGrafanaAnnotator{
		client:       grafana.NewGrafanaClient(grafanaURL, grafanaToken),
		dashboardUID: dashboardUID,
		logger:       logger,
	}
}

func (l SingleLineGrafanaAnnotator) OnChaosCreated(chaos Chaos) {
}

func (l SingleLineGrafanaAnnotator) OnChaosCreationFailed(chaos Chaos, reason error) {
}

func (l SingleLineGrafanaAnnotator) OnChaosStarted(chaos Chaos) {
	experiment, _ := chaos.GetExperimentStatus()
	duration, _ := chaos.GetChaosDuration()

	var sb strings.Builder
	sb.WriteString("<body>")
	sb.WriteString(fmt.Sprintf("<h4>%s Started</h4>", chaos.GetChaosTypeStr()))
	sb.WriteString(fmt.Sprintf("<div>Name: %s</div>", chaos.Object.GetName()))
	if chaos.Description != "" {
		sb.WriteString(fmt.Sprintf("<div>Description: %s</div>", chaos.Description))
	}
	sb.WriteString(fmt.Sprintf("<div>Start Time: %s</div>", chaos.GetStartTime().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("<div>Duration: %s</div>", duration.String()))

	spec := chaos.GetChaosSpec()
	specBytes, err := json.MarshalIndent(spec, "", "  ")
	if err == nil && len(specBytes) > 0 {
		sb.WriteString("<br>")
		sb.WriteString("<h5>Spec:</h5>")
		sb.WriteString(string(specBytes))
		sb.WriteString("<br>")
	} else {
		l.logger.Warn().Msgf("could not get chaos spec: %s", err)
	}

	if len(experiment.Records) > 0 {
		sb.WriteString("<br>")
		sb.WriteString("<h5>Records:</h5>")
		sb.WriteString("<ul>")
		for _, record := range experiment.Records {
			sb.WriteString(fmt.Sprintf("<li>%s: %s</li>", record.Id, record.Phase))
		}
		sb.WriteString("</ul>")
	}

	sb.WriteString("</body>")

	a := grafana.PostAnnotation{
		DashboardUID: l.dashboardUID,
		Time:         Ptr[time.Time](chaos.GetStartTime()),
		Text:         sb.String(),
	}
	_, resp, err := l.client.PostAnnotation(a)
	if err != nil {
		l.logger.Warn().Msgf("could not annotate on Grafana: %s", err)
	}
	l.logger.Debug().Any("GrafanaResponse", resp.String()).Msg("Annotated chaos experiment start")
}

func (l SingleLineGrafanaAnnotator) OnChaosPaused(chaos Chaos) {
}

func (l SingleLineGrafanaAnnotator) OnChaosEnded(chaos Chaos) {
	experiment, _ := chaos.GetExperimentStatus()
	duration, _ := chaos.GetChaosDuration()

	var sb strings.Builder
	sb.WriteString("<body>")
	sb.WriteString(fmt.Sprintf("<h4>%s Ended</h4>", chaos.GetChaosTypeStr()))
	sb.WriteString(fmt.Sprintf("<div>Name: %s</div>", chaos.Object.GetName()))
	if chaos.Description != "" {
		sb.WriteString(fmt.Sprintf("<div>Description: %s</div>", chaos.Description))
	}
	sb.WriteString(fmt.Sprintf("<div>Start Time: %s</div>", chaos.GetStartTime().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("<div>End Time: %s</div>", chaos.GetEndTime().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("<div>Duration: %s</div>", duration.String()))

	spec := chaos.GetChaosSpec()
	specBytes, err := json.MarshalIndent(spec, "", "  ")
	if err == nil && len(specBytes) > 0 {
		sb.WriteString("<br>")
		sb.WriteString("<h5>Spec:</h5>")
		sb.WriteString(string(specBytes))
		sb.WriteString("<br>")
	} else {
		l.logger.Warn().Msgf("could not get chaos spec: %s", err)
	}

	if len(experiment.Records) > 0 {
		sb.WriteString("<br>")
		sb.WriteString("<h5>Records:</h5>")
		sb.WriteString("<ul>")
		for _, record := range experiment.Records {
			sb.WriteString(fmt.Sprintf("<li>%s: %s</li>", record.Id, record.Phase))
		}
		sb.WriteString("</ul>")
	}

	sb.WriteString("</body>")

	a := grafana.PostAnnotation{
		DashboardUID: l.dashboardUID,
		Time:         Ptr[time.Time](chaos.GetEndTime()),
		Text:         sb.String(),
	}
	_, resp, err := l.client.PostAnnotation(a)
	if err != nil {
		l.logger.Warn().Msgf("could not annotate on Grafana: %s", err)
	}
	l.logger.Debug().Any("GrafanaResponse", resp.String()).Msg("Annotated chaos experiment end")
}

// OnChaosStatusUnknown handles the event when the status of a chaos experiment is unknown.
// It allows listeners to respond appropriately to this specific status change in the chaos lifecycle.
func (l SingleLineGrafanaAnnotator) OnChaosStatusUnknown(chaos Chaos) {}
