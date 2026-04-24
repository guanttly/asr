package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	appuser "github.com/lgt/asr/internal/application/user"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

type productCapabilitiesPayload struct {
	Realtime     bool `json:"realtime"`
	Batch        bool `json:"batch"`
	Meeting      bool `json:"meeting"`
	Voiceprint   bool `json:"voiceprint"`
	VoiceControl bool `json:"voice_control"`
}

type productFeaturesPayload struct {
	Edition      pkgconfig.ProductEdition   `json:"edition"`
	Capabilities productCapabilitiesPayload `json:"capabilities"`
}

type featureGate struct {
	features pkgconfig.ProductFeatures
}

func newFeatureGate(features pkgconfig.ProductFeatures) featureGate {
	return featureGate{features: features}
}

func (g featureGate) payload() productFeaturesPayload {
	return productFeaturesPayload{
		Edition: g.features.Edition,
		Capabilities: productCapabilitiesPayload{
			Realtime:     g.features.Realtime,
			Batch:        g.features.Batch,
			Meeting:      g.features.Meeting,
			Voiceprint:   g.features.Voiceprint,
			VoiceControl: g.features.VoiceControl,
		},
	}
}

func (g featureGate) meeting() bool {
	return g.features.Meeting
}

func (g featureGate) voiceprint() bool {
	return g.features.Voiceprint
}

func (g featureGate) voiceControl() bool {
	return g.features.VoiceControl
}

func (g featureGate) allowWorkflowType(workflowType wfdomain.WorkflowType) bool {
	switch workflowType {
	case wfdomain.WorkflowTypeMeeting:
		return g.meeting()
	case wfdomain.WorkflowTypeVoice:
		return g.voiceControl()
	default:
		return true
	}
}

func (g featureGate) allowNodeType(nodeType wfdomain.NodeType) bool {
	switch nodeType {
	case wfdomain.NodeMeetingSummary, wfdomain.NodeSpeakerDiarize:
		return g.meeting()
	case wfdomain.NodeVoiceWake, wfdomain.NodeVoiceIntent:
		return g.voiceControl()
	default:
		return true
	}
}

func (g featureGate) sanitizeWorkflowBindings(resp *appuser.WorkflowBindingsResponse) *appuser.WorkflowBindingsResponse {
	if resp == nil {
		return &appuser.WorkflowBindingsResponse{}
	}
	copy := *resp
	if !g.meeting() {
		copy.Meeting = nil
	}
	if !g.voiceControl() {
		copy.Voice = nil
	}
	return &copy
}

func (g featureGate) constrainWorkflowBindingsRequest(req *appuser.UpdateWorkflowBindingsRequest) error {
	if req == nil {
		return nil
	}
	if req.Meeting != nil && !g.meeting() {
		return fmt.Errorf("当前版本未开放会议纪要")
	}
	if req.Voice != nil && !g.voiceControl() {
		return fmt.Errorf("当前版本未开放终端语音控制")
	}
	return nil
}

func (g featureGate) denyFeature(c *gin.Context, message string) {
	response.Error(c, http.StatusForbidden, errcode.CodeForbidden, message)
}

func (g featureGate) denyWorkflowHidden(c *gin.Context) {
	response.Error(c, http.StatusNotFound, errcode.CodeNotFound, "workflow not found")
}
