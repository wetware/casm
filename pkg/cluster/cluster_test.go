package cluster_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	logtest "github.com/lthibault/log/test"
	mock_cluster "github.com/wetware/casm/internal/mock/pkg/cluster"
	"github.com/wetware/casm/pkg/cluster"

	"github.com/stretchr/testify/assert"
)

func TestRouter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var canceled bool
	defer func() {
		assert.True(t, canceled, "should cancel topic relay")
	}()

	logger := logtest.NewMockLogger(ctrl)
	logger.EXPECT().
		With(gomock.Any()).
		Return(logger).
		Times(1)

	table := mock_cluster.NewMockRoutingTable(ctrl)
	table.EXPECT().
		Advance(gomock.AssignableToTypeOf(time.Time{})).
		AnyTimes()

	topic := mock_cluster.NewMockTopic(ctrl)
	topic.EXPECT().
		String().
		Return("casm").
		AnyTimes()
	topic.EXPECT().
		Relay().
		Return(func() { canceled = true }, nil).
		Times(1)
	boot := topic.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)
	topic.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		After(boot).
		AnyTimes()

	router := cluster.Router{
		Log:          logger,
		Topic:        topic,
		RoutingTable: table,
	}
	defer router.Stop()

	err := router.Bootstrap(context.Background())
	assert.NoError(t, err, "bootstrap should succeed")
}
