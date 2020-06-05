package keeper_test

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant/keeper"
	"github.com/cosmos/cosmos-sdk/x/feegrant/types"
	"github.com/stretchr/testify/suite"
	abci "github.com/tendermint/tendermint/abci/types"
	"testing"
)

type KeeperTestSuite struct {
	suite.Suite

	cdc *codec.Codec
	ctx sdk.Context
	dk  keeper.Keeper

	addr  sdk.AccAddress
	addr2 sdk.AccAddress
	addr3 sdk.AccAddress
	addr4 sdk.AccAddress
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}

func (suite *KeeperTestSuite) SetupTest() {
	app := simapp.Setup(false)
	suite.ctx = app.BaseApp.NewContext(false, abci.Header{})
	suite.dk = app.FeeGrantKeeper
	suite.addr = mustAddr("cosmos157ez5zlaq0scm9aycwphhqhmg3kws4qusmekll")
	suite.addr2 = mustAddr("cosmos1rjxwm0rwyuldsg00qf5lt26wxzzppjzxs2efdw")
	suite.addr3 = mustAddr("cosmos1qk93t4j0yyzgqgt6k5qf8deh8fq6smpn3ntu3x")
	suite.addr4 = mustAddr("cosmos1p9qh4ldfd6n0qehujsal4k7g0e37kel90rc4ts")
}

func mustAddr(acc string) sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(acc)
	if err != nil {
		panic(err)
	}
	return addr
}

func (suite *KeeperTestSuite) TestKeeperCrud() {
	ctx := suite.ctx
	k := suite.dk

	// some helpers
	atom := sdk.NewCoins(sdk.NewInt64Coin("atom", 555))
	eth := sdk.NewCoins(sdk.NewInt64Coin("eth", 123))
	basic := &types.BasicFeeAllowance{
		SpendLimit: atom,
		Expiration: types.ExpiresAtHeight(334455),
	}
	basic2 := &types.BasicFeeAllowance{
		SpendLimit: eth,
		Expiration: types.ExpiresAtHeight(172436),
	}

	// let's set up some initial state here
	k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr, suite.addr2, basic))
	k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr, suite.addr3, basic2))
	k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr2, suite.addr3, basic))
	k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr2, suite.addr4, basic))
	k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr4, suite.addr, basic2))

	// remove some, overwrite other
	k.RevokeFeeAllowance(ctx, suite.addr, suite.addr2)
	k.RevokeFeeAllowance(ctx, suite.addr, suite.addr3)
	k.GetFeeAllowance(ctx, suite.addr, suite.addr3)

	k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr, suite.addr3, basic))

	k.GetFeeAllowance(ctx, suite.addr, suite.addr3)
	k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr2, suite.addr3, basic2))
	// end state:
	// addr -> addr3 (basic)
	// addr2 -> addr3 (basic2), addr4(basic)
	// addr4 -> addr (basic2)

	// then lots of queries
	cases := map[string]struct {
		grantee   sdk.AccAddress
		granter   sdk.AccAddress
		allowance types.FeeAllowanceI
	}{
		"addr revoked": {
			granter: suite.addr,
			grantee: suite.addr2,
		},
		"addr revoked and added": {
			granter:   suite.addr,
			grantee:   suite.addr3,
			allowance: basic,
		},
		"addr never there": {
			granter: suite.addr,
			grantee: suite.addr4,
		},
		"addr modified": {
			granter:   suite.addr2,
			grantee:   suite.addr3,
			allowance: basic2,
		},
	}

	for name, tc := range cases {
		tc := tc
		suite.Run(name, func() {
			allow := k.GetFeeAllowance(ctx, tc.granter, tc.grantee)
			if tc.allowance == nil {
				suite.Nil(allow)
				return
			}
			suite.NotNil(allow)
			suite.Equal(tc.allowance, allow)
		})
	}

	allCases := map[string]struct {
		grantee sdk.AccAddress
		grants  []types.FeeAllowanceGrant
	}{
		"addr2 has none": {
			grantee: suite.addr2,
		},
		"addr has one": {
			grantee: suite.addr,
			grants:  []types.FeeAllowanceGrant{types.NewFeeAllowanceGrant(suite.addr4, suite.addr, basic2)},
		},
		"addr3 has two": {
			grantee: suite.addr3,
			grants: []types.FeeAllowanceGrant{
				types.NewFeeAllowanceGrant(suite.addr, suite.addr3, basic),
				types.NewFeeAllowanceGrant(suite.addr2, suite.addr3, basic2),
			},
		},
	}

	for name, tc := range allCases {
		tc := tc
		suite.Run(name, func() {
			var grants []types.FeeAllowanceGrant
			err := k.IterateAllGranteeFeeAllowances(ctx, tc.grantee, func(grant types.FeeAllowanceGrant) bool {
				grants = append(grants, grant)
				return false
			})
			suite.NoError(err)
			suite.Equal(tc.grants, grants)
		})
	}
}

func (suite *KeeperTestSuite) TestUseGrantedFee() {
	ctx := suite.ctx
	k := suite.dk

	// some helpers
	atom := sdk.NewCoins(sdk.NewInt64Coin("atom", 555))
	eth := sdk.NewCoins(sdk.NewInt64Coin("eth", 123))
	future := &types.BasicFeeAllowance{
		SpendLimit: atom,
		Expiration: types.ExpiresAtHeight(5678),
	}

	expired := &types.BasicFeeAllowance{
		SpendLimit: eth,
		Expiration: types.ExpiresAtHeight(55),
	}

	// for testing limits of the contract
	hugeAtom := sdk.NewCoins(sdk.NewInt64Coin("atom", 9999))
	smallAtom := sdk.NewCoins(sdk.NewInt64Coin("atom", 1))
	futureAfterSmall := &types.BasicFeeAllowance{
		SpendLimit: sdk.NewCoins(sdk.NewInt64Coin("atom", 554)),
		Expiration: types.ExpiresAtHeight(5678),
	}

	// then lots of queries
	cases := map[string]struct {
		grantee sdk.AccAddress
		granter sdk.AccAddress
		fee     sdk.Coins
		allowed bool
		final   types.FeeAllowanceI
	}{
		"use entire pot": {
			granter: suite.addr,
			grantee: suite.addr2,
			fee:     atom,
			allowed: true,
			final:   nil,
		},
		"expired and removed": {
			granter: suite.addr,
			grantee: suite.addr3,
			fee:     eth,
			allowed: false,
			final:   nil,
		},
		"too high": {
			granter: suite.addr,
			grantee: suite.addr2,
			fee:     hugeAtom,
			allowed: false,
			final:   future,
		},
		"use a little": {
			granter: suite.addr,
			grantee: suite.addr2,
			fee:     smallAtom,
			allowed: true,
			final:   futureAfterSmall,
		},
	}

	for name, tc := range cases {
		tc := tc
		suite.Run(name, func() {
			// let's set up some initial state here
			// addr -> addr2 (future)
			// addr -> addr3 (expired)

			k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr, suite.addr2, future))
			k.GrantFeeAllowance(ctx, types.NewFeeAllowanceGrant(suite.addr, suite.addr3, expired))

			err := k.UseGrantedFees(ctx, tc.granter, tc.grantee, tc.fee)
			if tc.allowed {
				suite.NoError(err)
			} else {
				suite.Error(err)
			}

			loaded := k.GetFeeAllowance(ctx, tc.granter, tc.grantee)

			suite.Equal(tc.final, loaded)
		})
	}
}
