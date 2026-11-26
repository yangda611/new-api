package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The channel cost ratio must scale the group ratio so that every billing path
// (ratio, per-call price, tiered expr) settles at the channel's own price.
func TestHandleGroupRatioAppliesChannelCostRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ratio_setting.InitRatioSettings()

	newCtx := func(costRatio float64) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if costRatio > 0 {
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ChannelCostRatio: costRatio})
		}
		return c
	}
	info := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{UsingGroup: "default", UserGroup: "default"}
	}

	base := HandleGroupRatio(newCtx(0), info())
	require.InDelta(t, ratio_setting.GetGroupRatio("default"), base.GroupRatio, 1e-9)
	require.Zero(t, base.ChannelCostRatio)

	discounted := HandleGroupRatio(newCtx(0.5), info())
	require.InDelta(t, base.GroupRatio*0.5, discounted.GroupRatio, 1e-9)
	require.InDelta(t, 0.5, discounted.ChannelCostRatio, 1e-9)

	// A non-positive ratio means "not configured" and must stay neutral.
	neutral := HandleGroupRatio(newCtx(0), info())
	require.InDelta(t, base.GroupRatio, neutral.GroupRatio, 1e-9)
}
