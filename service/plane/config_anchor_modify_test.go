package plane

import (
	"testing"

	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/stretchr/testify/assert"
)

func Test_srv_getAnchorPerModifyStats(t *testing.T) {
	current := []*peer.AnchorPeer{
		{Host: "localhost", Port: 1234},
		{Host: "localhost", Port: 1235},
		{Host: "localhost", Port: 1236},
		{Host: "localhost", Port: 1237},
	}
	req := &proto.ConfigAnchorModifyRequest{
		ChannelName: "oops",
		Peers: []*peer.AnchorPeer{
			{Host: "localhost", Port: 1234},
			{Host: "localhost", Port: 1238},
			{Host: "localhost", Port: 1235},
		},
	}

	s := &srv{}
	existedPeers, newPeers, deletedPeers := s.getAnchorPerModifyStats(current, req)
	assert.Len(t, existedPeers, 2)
	assert.Len(t, newPeers, 1)
	assert.Len(t, deletedPeers, 2, "deleted peers check failed")
}
