package cscc

import (
	"context"
	"fmt"
	"sort"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/protoutil"
)

func (c *cli) GetChannels(ctx context.Context) ([]string, error) {
	// create chaincode endorsement proposal
	prop, err := c.createGetChannelsProposal()
	if err != nil {
		return nil, fmt.Errorf("create proposal: %w", err)
	}

	// create signed proposal using identity
	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return nil, fmt.Errorf("create singed proposal: %w", err)
	}

	// process signed proposal on peer
	channels, err := c.processGetChannelProposal(ctx, signedProp)
	if err != nil {
		return nil, fmt.Errorf("process proposal: %w", err)
	}

	// sort for channel list for pretty output
	sort.Strings(channels)

	return channels, nil
}

func (c *cli) createGetChannelsProposal() (*pb.Proposal, error) {
	var err error
	invocation := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
			ChaincodeId: &pb.ChaincodeID{Name: chaincodeName},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte(cscc.GetChannels)}},
		},
	}

	cr, err := c.id.Serialize()
	if err != nil {
		return nil, fmt.Errorf("signer serialize: %w", err)
	}
	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "", invocation, cr)
	if err != nil {
		return nil, fmt.Errorf("cannot create proposal, due to %w", err)
	}
	return prop, nil
}

func (c *cli) processGetChannelProposal(ctx context.Context, prop *pb.SignedProposal) ([]string, error) {
	resp, err := c.cli.ProcessProposal(ctx, prop)
	if err != nil {
		return nil, fmt.Errorf("endorse proposal: %w", err)
	}
	if resp.Response == nil || resp.Response.Status != 200 {
		return nil, fmt.Errorf("received bad response, status %d: %s", resp.Response.Status, resp.Response.Message)
	}

	var channelQueryResponse pb.ChannelQueryResponse
	if err = proto.Unmarshal(resp.Response.Payload, &channelQueryResponse); err != nil {
		return nil, fmt.Errorf("unmarshal proto: %w", err)
	}

	channels := make([]string, 0, len(channelQueryResponse.Channels))
	for _, ch := range channelQueryResponse.Channels {
		channels = append(channels, ch.ChannelId)
	}
	return channels, nil
}
