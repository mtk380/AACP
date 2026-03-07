package state

import "fmt"

const (
	PrefixAMXListing   = "amx/l/"
	PrefixAMXOrder     = "amx/o/"
	PrefixAMXIndex     = "amx/i/"
	PrefixAAPTask      = "aap/t/"
	PrefixAAPEvidence  = "aap/e/"
	PrefixAAPSLA       = "aap/s/"
	PrefixAAPHeartbeat = "aap/h/"
	PrefixWeaveDAG     = "weave/d/"
	PrefixWeaveNode    = "weave/n/"
	PrefixCapUTXO      = "cap/u/"
	PrefixCapHolder    = "cap/h/"
	PrefixCapType      = "cap/t/"
	PrefixCapDerive    = "cap/d/"
	PrefixRepScore     = "rep/s/"
	PrefixArbDispute   = "arb/d/"
	PrefixArbVote      = "arb/v/"
	PrefixFiatEscrow   = "fiat/e/"
	PrefixFiatDeposit  = "fiat/dep/"
	PrefixFiatPool     = "fiat/pool"
	PrefixNodeReg      = "node/r/"
	PrefixNodeVal      = "node/v/"
	PrefixGovProp      = "gov/p/"
	PrefixGovVote      = "gov/v/"
	PrefixAccNonce     = "acc/n/"
	PrefixParams       = "params/"
)

func ListingKey(id string) []byte { return []byte(PrefixAMXListing + id) }
func OrderKey(id string) []byte   { return []byte(PrefixAMXOrder + id) }
func TaskKey(id string) []byte    { return []byte(PrefixAAPTask + id) }
func UTXOKey(id string) []byte    { return []byte(PrefixCapUTXO + id) }
func RepKey(entity string) []byte { return []byte(PrefixRepScore + entity) }

func NonceKey(pubHex string) []byte {
	return []byte(PrefixAccNonce + pubHex)
}

func ParamKey(module, key string) []byte {
	return []byte(fmt.Sprintf("%s%s/%s", PrefixParams, module, key))
}

func HeartbeatKey(taskID string, seq uint64) []byte {
	return []byte(fmt.Sprintf("%s%s/%010d", PrefixAAPHeartbeat, taskID, seq))
}

func WeaveNodeKey(dagID, nodeID string) []byte {
	return []byte(fmt.Sprintf("%s%s/%s", PrefixWeaveNode, dagID, nodeID))
}
