package havoc

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/grafana"
)

type RangeGrafanaAnnotator struct {
	client       *grafana.Client
	dashboardUID string
	chaosMap     map[string]int64 // Maps Chaos ID to Grafana Annotation ID
	logger       zerolog.Logger
}

func NewRangeGrafanaAnnotator(grafanaURL, grafanaToken, dashboardUID string, logger zerolog.Logger) *RangeGrafanaAnnotator {
	return &RangeGrafanaAnnotator{
		client:       grafana.NewGrafanaClient(grafanaURL, grafanaToken),
		dashboardUID: dashboardUID,
		chaosMap:     make(map[string]int64),
		logger:       logger,
	}
}

func (l RangeGrafanaAnnotator) OnChaosCreated(chaos Chaos) {}

func (l RangeGrafanaAnnotator) OnChaosCreationFailed(chaos Chaos, reason error) {}

func (l RangeGrafanaAnnotator) OnChaosStarted(chaos Chaos) {
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
	res, resp, err := l.client.PostAnnotation(a)
	if err != nil {
		l.logger.Warn().Msgf("could not annotate on Grafana: %s", err)
	}
	l.logger.Debug().Any("GrafanaResponse", resp.String()).Msg("Annotated chaos experiment start")
	l.chaosMap[chaos.GetChaosName()] = res.ID
}

func (l RangeGrafanaAnnotator) OnChaosPaused(chaos Chaos) {
}

func (l RangeGrafanaAnnotator) OnChaosEnded(chaos Chaos) {
	annotationID, exists := l.chaosMap[chaos.GetChaosName()]
	if !exists {
		l.logger.Error().Msgf("No Grafana annotation ID found for Chaos: %s", chaos.GetChaosName())
		return
	}

	experiment, _ := chaos.GetExperimentStatus()
	duration, _ := chaos.GetChaosDuration()

	var sb strings.Builder
	sb.WriteString("<body>")
	sb.WriteString(fmt.Sprintf("<h4>%s</h4>", chaos.GetChaosTypeStr()))
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

	// Delete the temporary start annotation
	_, err = l.client.DeleteAnnotation(annotationID)
	if err != nil {
		l.logger.Error().Msgf("could not delete temporary start annotation: %s", err)
	}
	delete(l.chaosMap, chaos.GetChaosName())

	// Create the final annotation (time range)
	a := grafana.PostAnnotation{
		DashboardUID: l.dashboardUID,
		Time:         Ptr[time.Time](chaos.GetStartTime()),
		TimeEnd:      Ptr[time.Time](chaos.GetEndTime()),
		Text:         sb.String(),
	}
	res, resp, err := l.client.PostAnnotation(a)
	if err != nil {
		l.logger.Warn().Msgf("could not annotate on Grafana: %s", err)
	}
	l.logger.Debug().Any("GrafanaResponse", resp.String()).Msg("Annotated chaos experiment end")
	l.chaosMap[chaos.GetChaosName()] = res.ID
}

// OnChaosStatusUnknown handles the event when the status of a chaos experiment is unknown.
// It allows listeners to respond appropriately to this specific status change in the chaos lifecycle.
func (l RangeGrafanaAnnotator) OnChaosStatusUnknown(chaos Chaos) {}
