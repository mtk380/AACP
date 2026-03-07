package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"aacp/internal/abci"
	aacpcrypto "aacp/internal/crypto"
	"aacp/internal/state"
	"aacp/internal/types"
)

type healthResp struct {
	Status    string `json:"status"`
	AppHash   string `json:"app_hash"`
	Height    int64  `json:"height"`
	Timestamp int64  `json:"timestamp"`
}

type finalizeReq struct {
	Txs [][]byte `json:"txs"`
}

type finalizeHexReq struct {
	TxsHex []string `json:"txs_hex"`
}

type caputxoDemoResp struct {
	ParentUTXOID string                      `json:"parent_utxo_id"`
	Mint         *abci.FinalizeBlockResponse `json:"mint"`
	Delegate     *abci.FinalizeBlockResponse `json:"delegate"`
	Revoke       *abci.FinalizeBlockResponse `json:"revoke"`
	Health       healthResp                  `json:"health"`
}

type stateEntry struct {
	Key         string `json:"key"`
	ValueBase64 string `json:"value_base64"`
	ValueText   string `json:"value_text,omitempty"`
	ValueJSON   any    `json:"value_json,omitempty"`
}

type statePrefixResp struct {
	Prefix  string       `json:"prefix"`
	Limit   int          `json:"limit"`
	Count   int          `json:"count"`
	Height  int64        `json:"height"`
	Entries []stateEntry `json:"entries"`
}

type statePrefixesResp struct {
	Prefixes []string `json:"prefixes"`
}

type capabilityAction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type capabilityModule struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	KeyPrefixes []string           `json:"key_prefixes"`
	Actions     []capabilityAction `json:"actions"`
}

type capabilitiesResp struct {
	Project       string             `json:"project"`
	Purpose       string             `json:"purpose"`
	QuickStart    []string           `json:"quick_start"`
	WhatYouCanDo  []string           `json:"what_you_can_do"`
	Modules       []capabilityModule `json:"modules"`
	StatePrefixes []string           `json:"state_prefixes"`
}

type lifeActor struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	Address string `json:"address"`
}

type lifeImplementation struct {
	File     string `json:"file"`
	Function string `json:"function"`
	Behavior string `json:"behavior"`
}

type lifeTxOutcome struct {
	Code    uint32        `json:"code"`
	Log     string        `json:"log,omitempty"`
	GasUsed uint64        `json:"gas_used,omitempty"`
	Events  []types.Event `json:"events,omitempty"`
}

type lifeStateSnapshot struct {
	Listing map[string]any `json:"listing,omitempty"`
	Order   map[string]any `json:"order,omitempty"`
	Escrow  map[string]any `json:"escrow,omitempty"`
	Task    map[string]any `json:"task,omitempty"`
	Dispute map[string]any `json:"dispute,omitempty"`
}

type lifeStep struct {
	No             int                `json:"no"`
	Title          string             `json:"title"`
	Why            string             `json:"why"`
	Module         string             `json:"module"`
	Action         string             `json:"action"`
	Implementation lifeImplementation `json:"implementation"`
	Sender         string             `json:"sender"`
	Nonce          uint64             `json:"nonce"`
	Input          any                `json:"input"`
	TxHash         string             `json:"tx_hash"`
	Outcome        lifeTxOutcome      `json:"outcome"`
	Success        bool               `json:"success"`
	Height         int64              `json:"height"`
	AppHash        string             `json:"app_hash"`
	StateBefore    lifeStateSnapshot  `json:"state_before"`
	StateAfter     lifeStateSnapshot  `json:"state_after"`
}

type lifeIDs struct {
	ListingID string `json:"listing_id"`
	OrderID   string `json:"order_id"`
	EscrowID  string `json:"escrow_id"`
	TaskID    string `json:"task_id"`
	DisputeID string `json:"dispute_id"`
}

type lifeState struct {
	OrderStatus   string `json:"order_status"`
	TaskStatus    string `json:"task_status"`
	EscrowStatus  string `json:"escrow_status"`
	DisputeStatus string `json:"dispute_status"`
	DisputeWinner string `json:"dispute_winner"`
}

type lifeStoryResp struct {
	Scenario    string      `json:"scenario"`
	WhyUseAACP  []string    `json:"why_use_aacp"`
	WithoutAACP []string    `json:"without_aacp"`
	WithAACP    []string    `json:"with_aacp"`
	Actors      []lifeActor `json:"actors"`
	Steps       []lifeStep  `json:"steps"`
	IDs         lifeIDs     `json:"ids"`
	FinalState  lifeState   `json:"final_state"`
	Summary     string      `json:"summary"`
	NextTry     []string    `json:"next_try"`
}

//go:embed ui/*
var uiFiles embed.FS

func main() {
	var (
		host = flag.String("host", "0.0.0.0", "http listen host")
		port = flag.Int("port", 8888, "http listen port")
	)
	flag.Parse()

	app := abci.NewApp()
	if _, err := app.InitChain(&abci.InitChainRequest{Genesis: map[string][]byte{}}); err != nil {
		log.Fatalf("init chain failed: %v", err)
	}

	var (
		height int64
		mu     sync.Mutex
	)

	finalize := func(txs [][]byte) (*abci.FinalizeBlockResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		height++
		return app.FinalizeBlock(&abci.FinalizeBlockRequest{
			Height: height,
			Time:   time.Now(),
			Txs:    txs,
		})
	}

	currentHealth := func() healthResp {
		mu.Lock()
		defer mu.Unlock()
		return healthResp{
			Status:    "ok",
			AppHash:   app.LastAppHashHex(),
			Height:    height,
			Timestamp: time.Now().Unix(),
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, currentHealth())
	})
	mux.HandleFunc("/api/finalize-empty", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp, err := finalize(nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, resp)
	})
	mux.HandleFunc("/api/finalize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req finalizeReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := finalize(req.Txs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, resp)
	})
	mux.HandleFunc("/api/finalize-hex", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req finalizeHexReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		txs := make([][]byte, 0, len(req.TxsHex))
		for i, txHex := range req.TxsHex {
			tx, err := decodeHexString(txHex)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid tx hex at index %d: %v", i, err), http.StatusBadRequest)
				return
			}
			txs = append(txs, tx)
		}
		resp, err := finalize(txs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, resp)
	})
	mux.HandleFunc("/api/demo/caputxo-full", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		issuer, err := aacpcrypto.GenerateKeyPair()
		if err != nil {
			http.Error(w, fmt.Sprintf("generate issuer key: %v", err), http.StatusInternalServerError)
			return
		}
		holder, err := aacpcrypto.GenerateKeyPair()
		if err != nil {
			http.Error(w, fmt.Sprintf("generate holder key: %v", err), http.StatusInternalServerError)
			return
		}
		delegate, err := aacpcrypto.GenerateKeyPair()
		if err != nil {
			http.Error(w, fmt.Sprintf("generate delegate key: %v", err), http.StatusInternalServerError)
			return
		}

		parentUTXOID := mintUTXOID(issuer.PubKey, "text_generation", 0)

		txMint, err := signedTx(
			issuer.PubKey,
			issuer.PrivKey,
			0,
			"caputxo",
			"mint_capability",
			map[string]any{
				"holder":      holder.PubKey,
				"cap_type":    "text_generation",
				"cap_version": "1.0.0",
				"scopes":      []string{"read", "write"},
				"delegatable": true,
				"max_depth":   3,
			},
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("build mint tx: %v", err), http.StatusInternalServerError)
			return
		}
		mintResp, err := finalize([][]byte{txMint})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(mintResp.TxResults) == 0 || mintResp.TxResults[0].Code != 0 {
			writeJSON(w, caputxoDemoResp{
				ParentUTXOID: parentUTXOID,
				Mint:         mintResp,
				Health:       currentHealth(),
			})
			return
		}

		txDelegate, err := signedTx(
			holder.PubKey,
			holder.PrivKey,
			0,
			"caputxo",
			"delegate_capability",
			map[string]any{
				"parent_utxo_id": parentUTXOID,
				"new_holder":     delegate.PubKey,
				"scopes":         []string{"read"},
			},
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("build delegate tx: %v", err), http.StatusInternalServerError)
			return
		}
		delegateResp, err := finalize([][]byte{txDelegate})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(delegateResp.TxResults) == 0 || delegateResp.TxResults[0].Code != 0 {
			writeJSON(w, caputxoDemoResp{
				ParentUTXOID: parentUTXOID,
				Mint:         mintResp,
				Delegate:     delegateResp,
				Health:       currentHealth(),
			})
			return
		}

		txRevoke, err := signedTx(
			issuer.PubKey,
			issuer.PrivKey,
			1,
			"caputxo",
			"revoke_capability",
			map[string]any{
				"utxo_id": parentUTXOID,
			},
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("build revoke tx: %v", err), http.StatusInternalServerError)
			return
		}
		revokeResp, err := finalize([][]byte{txRevoke})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, caputxoDemoResp{
			ParentUTXOID: parentUTXOID,
			Mint:         mintResp,
			Delegate:     delegateResp,
			Revoke:       revokeResp,
			Health:       currentHealth(),
		})
	})
	mux.HandleFunc("/api/state/prefix", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		prefix := r.URL.Query().Get("prefix")
		if prefix == "" {
			http.Error(w, "prefix is required", http.StatusBadRequest)
			return
		}
		limit, err := parseLimit(r.URL.Query().Get("limit"), 50, 1, 500)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		entries := app.Store().IteratePrefix([]byte(prefix))
		if len(entries) > limit {
			entries = entries[:limit]
		}
		out := make([]stateEntry, 0, len(entries))
		for _, kv := range entries {
			entry := stateEntry{
				Key:         string(kv.Key),
				ValueBase64: base64.StdEncoding.EncodeToString(kv.Value),
			}
			if utf8.Valid(kv.Value) {
				entry.ValueText = string(kv.Value)
			}
			var parsed any
			if err := json.Unmarshal(kv.Value, &parsed); err == nil {
				entry.ValueJSON = parsed
			}
			out = append(out, entry)
		}

		mu.Lock()
		currentHeight := height
		mu.Unlock()
		writeJSON(w, statePrefixResp{
			Prefix:  prefix,
			Limit:   limit,
			Count:   len(out),
			Height:  currentHeight,
			Entries: out,
		})
	})
	mux.HandleFunc("/api/state/prefixes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, statePrefixesResp{
			Prefixes: knownStatePrefixes(),
		})
	})
	mux.HandleFunc("/api/meta/capabilities", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, buildCapabilitiesDoc())
	})
	mux.HandleFunc("/api/demo/story-poster", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		provider, err := aacpcrypto.GenerateKeyPair()
		if err != nil {
			http.Error(w, fmt.Sprintf("generate provider key: %v", err), http.StatusInternalServerError)
			return
		}
		consumer, err := aacpcrypto.GenerateKeyPair()
		if err != nil {
			http.Error(w, fmt.Sprintf("generate consumer key: %v", err), http.StatusInternalServerError)
			return
		}

		var providerNonce uint64
		var consumerNonce uint64
		storySteps := make([]lifeStep, 0, 10)
		title := "奶茶店活动海报设计（24小时交付）"

		listingID := shortHashID(provider.PubKey, []byte(fmt.Sprintf("%d", providerNonce)), []byte(title))
		orderID := shortHashID(consumer.PubKey, []byte(fmt.Sprintf("%d", consumerNonce)), []byte(listingID))
		escrowID := "esc-" + orderID
		taskID := taskIDFromOrder(orderID, 3)
		disputeID := disputeIDForDemo(orderID, taskID, 5)
		storyIDs := lifeIDs{
			ListingID: listingID,
			OrderID:   orderID,
			EscrowID:  escrowID,
			TaskID:    taskID,
			DisputeID: disputeID,
		}

		runStoryStep := func(no int, title, why, module, action string, senderPub ed25519.PublicKey, senderPriv ed25519.PrivateKey, nonce uint64, payload any) bool {
			impl := storyImplementation(module, action)
			stateBefore := snapshotStoryState(app.Store(), storyIDs)
			sender := shortAddress(senderPub)

			rawTx, err := signedTx(senderPub, senderPriv, nonce, module, action, payload)
			if err != nil {
				current := currentHealth()
				storySteps = append(storySteps, lifeStep{
					No:             no,
					Title:          title,
					Why:            why,
					Module:         module,
					Action:         action,
					Implementation: impl,
					Sender:         sender,
					Nonce:          nonce,
					Input:          payload,
					Outcome:        lifeTxOutcome{Code: types.CodeDecodeFail, Log: err.Error()},
					Success:        false,
					Height:         current.Height,
					AppHash:        current.AppHash,
					StateBefore:    stateBefore,
					StateAfter:     snapshotStoryState(app.Store(), storyIDs),
				})
				return false
			}
			resp, err := finalize([][]byte{rawTx})
			if err != nil {
				current := currentHealth()
				storySteps = append(storySteps, lifeStep{
					No:             no,
					Title:          title,
					Why:            why,
					Module:         module,
					Action:         action,
					Implementation: impl,
					Sender:         sender,
					Nonce:          nonce,
					Input:          payload,
					TxHash:         txHashHex(rawTx),
					Outcome:        lifeTxOutcome{Code: types.CodeUnknownMod, Log: err.Error()},
					Success:        false,
					Height:         current.Height,
					AppHash:        current.AppHash,
					StateBefore:    stateBefore,
					StateAfter:     snapshotStoryState(app.Store(), storyIDs),
				})
				return false
			}
			outcome := firstTxOutcome(resp)
			current := currentHealth()
			storySteps = append(storySteps, lifeStep{
				No:             no,
				Title:          title,
				Why:            why,
				Module:         module,
				Action:         action,
				Implementation: impl,
				Sender:         sender,
				Nonce:          nonce,
				Input:          payload,
				TxHash:         txHashHex(rawTx),
				Outcome:        outcome,
				Success:        outcome.Code == 0,
				Height:         current.Height,
				AppHash:        current.AppHash,
				StateBefore:    stateBefore,
				StateAfter:     snapshotStoryState(app.Store(), storyIDs),
			})
			return outcome.Code == 0
		}

		ok := runStoryStep(
			1,
			"设计师发布服务",
			"把服务标准和价格写到链上，避免口头约定扯皮。",
			"amx",
			"create_listing",
			provider.PubKey,
			provider.PrivKey,
			providerNonce,
			map[string]any{
				"title":       title,
				"description": "为奶茶店设计春季促销海报，含2次修改",
				"capabilities": []map[string]any{
					{"cap_type": "poster_design"},
				},
				"pricing": map[string]any{
					"currency":   "CNY",
					"base_price": "199",
				},
				"sla": map[string]any{
					"max_latency_ms": 3000,
					"timeout_sec":    86400,
				},
				"tags": []string{"设计", "海报", "门店"},
			},
		)
		providerNonce++
		if ok {
			ok = runStoryStep(
				2,
				"老板发起需求并自动匹配",
				"系统按能力+预算自动找到合适服务，不用满世界问人。",
				"amx",
				"submit_request",
				consumer.PubKey,
				consumer.PrivKey,
				consumerNonce,
				map[string]any{
					"required_caps": []string{"poster_design"},
					"budget": map[string]any{
						"currency": "CNY",
						"amount":   "299",
					},
					"max_latency_ms": 5000,
					"deadline":       time.Now().Add(2 * time.Hour).Unix(),
				},
			)
		}
		consumerNonce++
		if ok {
			ok = runStoryStep(
				3,
				"老板先创建托管单",
				"先托管资金，再开工，降低‘收钱不干活/干活不给钱’风险。",
				"fiat",
				"create_escrow",
				consumer.PubKey,
				consumer.PrivKey,
				consumerNonce,
				map[string]any{
					"escrow_id": escrowID,
					"order_id":  orderID,
					"consumer":  consumer.PubKey,
					"provider":  provider.PubKey,
					"service_fee": map[string]any{
						"currency": "CNY",
						"amount":   "199",
					},
					"commission_bps": 1200,
				},
			)
		}
		consumerNonce++
		if ok {
			ok = runStoryStep(
				4,
				"老板给托管单打款",
				"款项锁定到托管状态，流程更可控。",
				"fiat",
				"fund_escrow",
				consumer.PubKey,
				consumer.PrivKey,
				consumerNonce,
				map[string]any{
					"escrow_id": escrowID,
				},
			)
		}
		consumerNonce++
		if ok {
			ok = runStoryStep(
				5,
				"系统创建任务单",
				"把双方身份、交付要求、重试次数写成可追踪任务。",
				"aap",
				"create_task",
				consumer.PubKey,
				consumer.PrivKey,
				consumerNonce,
				map[string]any{
					"order_id":    orderID,
					"consumer":    consumer.PubKey,
					"provider":    provider.PubKey,
					"max_retries": 1,
					"service_fee": map[string]any{
						"currency": "CNY",
						"amount":   "199",
					},
					"timeout_sec":    86400,
					"max_latency_ms": 5000,
					"min_accuracy":   0.9,
				},
			)
		}
		consumerNonce++
		if ok {
			ok = runStoryStep(
				6,
				"设计师提交作品",
				"交付被记录，后续验收有证据可查。",
				"aap",
				"submit_result",
				provider.PubKey,
				provider.PrivKey,
				providerNonce,
				map[string]any{
					"task_id":      taskID,
					"result_data":  []byte("poster-v1"),
					"result_ipfs":  "ipfs://demo/poster-v1",
					"content_hash": "poster-hash-v1",
				},
			)
		}
		providerNonce++
		if ok {
			ok = runStoryStep(
				7,
				"老板验收不通过",
				"不满意可明确拒绝，系统进入可仲裁状态。",
				"aap",
				"verify_result",
				consumer.PubKey,
				consumer.PrivKey,
				consumerNonce,
				map[string]any{
					"task_id":  taskID,
					"passed":   false,
					"reason":   "海报关键信息有误",
					"verifier": consumer.PubKey,
				},
			)
		}
		consumerNonce++
		if ok {
			ok = runStoryStep(
				8,
				"老板发起争议",
				"把纠纷及理由写入链上，避免各说各话。",
				"arb",
				"file_dispute",
				consumer.PubKey,
				consumer.PrivKey,
				consumerNonce,
				map[string]any{
					"order_id":    orderID,
					"task_id":     taskID,
					"defendant":   provider.PubKey,
					"reason_code": "quality",
					"description": "交付结果与门店需求不一致",
				},
			)
		}
		consumerNonce++
		if ok {
			ok = runStoryStep(
				9,
				"系统自动仲裁",
				"按规则先自动裁决，快于纯人工协调。",
				"arb",
				"auto_resolve",
				consumer.PubKey,
				consumer.PrivKey,
				consumerNonce,
				map[string]any{
					"dispute_id": disputeID,
				},
			)
		}
		consumerNonce++
		if ok {
			ok = runStoryStep(
				10,
				"托管退款给老板",
				"资金回流路径固定，减少执行争议。",
				"fiat",
				"refund_escrow",
				consumer.PubKey,
				consumer.PrivKey,
				consumerNonce,
				map[string]any{
					"escrow_id": escrowID,
				},
			)
		}

		final := lifeState{
			OrderStatus:   readStatusField(app.Store(), state.OrderKey(orderID)),
			TaskStatus:    readStatusField(app.Store(), state.TaskKey(taskID)),
			EscrowStatus:  readStatusField(app.Store(), []byte(state.PrefixFiatEscrow+escrowID)),
			DisputeStatus: readStatusField(app.Store(), []byte(state.PrefixArbDispute+disputeID)),
			DisputeWinner: readNestedStringField(app.Store(), []byte(state.PrefixArbDispute+disputeID), "verdict", "winner"),
		}

		summary := "演示已完成：需求、托管、交付、争议和退款都形成了可追踪链路。"
		if !ok {
			summary = "演示中断：某一步失败，请查看 steps 里的 code。"
		}

		writeJSON(w, lifeStoryResp{
			Scenario: "奶茶店老板找设计师做活动海报",
			WhyUseAACP: []string{
				"把“谁答应了什么、什么时候做了什么”变成可追溯记录。",
				"把钱放进托管流程，降低先款先货互不信任问题。",
				"交付不满意时，有统一争议处理与执行路径。",
			},
			WithoutAACP: []string{
				"聊天记录分散，责任界限不清。",
				"付款与交付靠信任，风险高。",
				"纠纷常靠拉扯，耗时且没有统一证据链。",
			},
			WithAACP: []string{
				"每一步都有链上状态可查。",
				"托管状态明确（已创建/已打款/已退款等）。",
				"争议可自动/半自动裁决，并落到可执行动作。",
			},
			Actors: []lifeActor{
				{Name: "小王", Role: "奶茶店老板（需求方）", Address: shortAddress(consumer.PubKey)},
				{Name: "小李", Role: "设计师（服务方）", Address: shortAddress(provider.PubKey)},
			},
			Steps:      storySteps,
			IDs:        storyIDs,
			FinalState: final,
			Summary:    summary,
			NextTry: []string{
				"在页面里查询前缀 cap/u/、amx/o/、fiat/e/、arb/d/ 看状态明细。",
				"把验收改成 passed=true，观察流程走向完成结算。",
				"把 reason_code 改成 timeout/payment，比较仲裁结果变化。",
			},
		})
	})

	uiFS, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		log.Fatalf("load embedded ui failed: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(uiFS)))

	server := &http.Server{Addr: fmt.Sprintf("%s:%d", *host, *port), Handler: mux}
	go func() {
		log.Printf("aacpd started on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen failed: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Printf("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func signedTx(senderPub ed25519.PublicKey, senderPriv ed25519.PrivateKey, nonce uint64, module, action string, payload any) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	env := &types.TxEnvelope{
		Sender:   senderPub,
		Nonce:    nonce,
		GasLimit: 1_000_000,
		GasPrice: 1,
		Module:   module,
		Action:   action,
		Payload:  payloadBytes,
	}
	signBytes, err := env.SignBytes()
	if err != nil {
		return nil, err
	}
	env.Signature = aacpcrypto.SignMessage(senderPriv, signBytes)
	return types.EncodeTx(env)
}

func decodeHexString(v string) ([]byte, error) {
	return hex.DecodeString(strings.TrimSpace(v))
}

func mintUTXOID(issuer []byte, capType string, nonce uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%x|%s|%d", issuer, capType, nonce)))
	return fmt.Sprintf("%x", h[:8])
}

func parseLimit(raw string, defaultValue, minValue, maxValue int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit %q", raw)
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("limit must be between %d and %d", minValue, maxValue)
	}
	return value, nil
}

func firstTxCode(resp *abci.FinalizeBlockResponse) uint32 {
	if resp == nil || len(resp.TxResults) == 0 || resp.TxResults[0] == nil {
		return types.CodeDecodeFail
	}
	return resp.TxResults[0].Code
}

func firstTxOutcome(resp *abci.FinalizeBlockResponse) lifeTxOutcome {
	if resp == nil || len(resp.TxResults) == 0 || resp.TxResults[0] == nil {
		return lifeTxOutcome{
			Code: types.CodeDecodeFail,
			Log:  "missing tx result",
		}
	}
	result := resp.TxResults[0]
	return lifeTxOutcome{
		Code:    result.Code,
		Log:     result.Log,
		GasUsed: result.GasUsed,
		Events:  result.Events,
	}
}

func txHashHex(rawTx []byte) string {
	if len(rawTx) == 0 {
		return ""
	}
	h := sha256.Sum256(rawTx)
	return hex.EncodeToString(h[:])
}

func storyImplementation(module, action string) lifeImplementation {
	switch module + "/" + action {
	case "amx/create_listing":
		return lifeImplementation{
			File:     "internal/modules/amx/module.go",
			Function: "createListing",
			Behavior: "生成 listing 并写入 amx/l/，同时建立能力索引 amx/i/。",
		}
	case "amx/submit_request":
		return lifeImplementation{
			File:     "internal/modules/amx/module.go",
			Function: "submitRequest",
			Behavior: "按能力和预算筛选服务，写入匹配订单到 amx/o/。",
		}
	case "fiat/create_escrow":
		return lifeImplementation{
			File:     "internal/modules/fiat/module.go",
			Function: "createEscrow",
			Behavior: "创建托管单并计算服务费+佣金，写入 fiat/e/。",
		}
	case "fiat/fund_escrow":
		return lifeImplementation{
			File:     "internal/modules/fiat/module.go",
			Function: "fundEscrow",
			Behavior: "将托管单状态从 pending 更新为 funded。",
		}
	case "aap/create_task":
		return lifeImplementation{
			File:     "internal/modules/aap/module.go",
			Function: "createTask",
			Behavior: "根据订单创建任务，初始化 SLA 与重试策略，写入 aap/t/。",
		}
	case "aap/submit_result":
		return lifeImplementation{
			File:     "internal/modules/aap/module.go",
			Function: "submitResult",
			Behavior: "记录交付物 hash/IPFS，任务进入提交结果状态。",
		}
	case "aap/verify_result":
		return lifeImplementation{
			File:     "internal/modules/aap/module.go",
			Function: "verifyResult",
			Behavior: "验收结果，失败时进入授权重试或争议状态。",
		}
	case "arb/file_dispute":
		return lifeImplementation{
			File:     "internal/modules/arb/module.go",
			Function: "fileDispute",
			Behavior: "创建争议单，记录原告/被告、原因和描述，写入 arb/d/。",
		}
	case "arb/auto_resolve":
		return lifeImplementation{
			File:     "internal/modules/arb/module.go",
			Function: "autoResolve",
			Behavior: "依据规则自动裁决，更新争议状态与 verdict。",
		}
	case "fiat/refund_escrow":
		return lifeImplementation{
			File:     "internal/modules/fiat/module.go",
			Function: "refundEscrow",
			Behavior: "按托管状态校验并执行退款，状态变更为 refunded。",
		}
	default:
		return lifeImplementation{
			File:     "internal/modules/" + module + "/module.go",
			Function: action,
			Behavior: "执行模块动作并更新对应状态前缀。",
		}
	}
}

func snapshotStoryState(store *state.Store, ids lifeIDs) lifeStateSnapshot {
	snapshot := lifeStateSnapshot{}

	if ids.ListingID != "" {
		listing := pickFields(
			readStateObject(store, state.ListingKey(ids.ListingID)),
			"listing_id", "title", "status", "pricing", "sla", "updated_at",
		)
		if len(listing) > 0 {
			snapshot.Listing = listing
		}
	}

	if ids.OrderID != "" {
		order := pickFields(
			readStateObject(store, state.OrderKey(ids.OrderID)),
			"order_id", "listing_id", "status", "agreed_price", "commission_bps", "commission_amount", "escrow_id", "task_id", "updated_at",
		)
		if len(order) > 0 {
			snapshot.Order = order
		}
	}

	if ids.EscrowID != "" {
		escrow := pickFields(
			readStateObject(store, []byte(state.PrefixFiatEscrow+ids.EscrowID)),
			"escrow_id", "order_id", "status", "service_fee", "commission", "total_amount", "funded_at", "released_at", "created_at",
		)
		if len(escrow) > 0 {
			snapshot.Escrow = escrow
		}
	}

	if ids.TaskID != "" {
		task := pickFields(
			readStateObject(store, state.TaskKey(ids.TaskID)),
			"task_id", "order_id", "status", "retry_count", "max_retries", "result_ipfs", "result_hash", "created_at", "completed_at",
		)
		if len(task) > 0 {
			snapshot.Task = task
		}
	}

	if ids.DisputeID != "" {
		dispute := pickFields(
			readStateObject(store, []byte(state.PrefixArbDispute+ids.DisputeID)),
			"dispute_id", "order_id", "task_id", "status", "phase", "reason_code", "verdict", "filed_at", "resolved_at",
		)
		if len(dispute) > 0 {
			snapshot.Dispute = dispute
		}
	}

	return snapshot
}

func readStateObject(store *state.Store, key []byte) map[string]any {
	raw, ok := store.Get(key)
	if !ok || len(raw) == 0 {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}
	return data
}

func pickFields(source map[string]any, fields ...string) map[string]any {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := source[field]; ok {
			out[field] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shortHashID(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

func taskIDFromOrder(orderID string, nonce uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", orderID, nonce)))
	return hex.EncodeToString(h[:8])
}

func disputeIDForDemo(orderID, taskID string, nonce uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", orderID, taskID, nonce)))
	return hex.EncodeToString(h[:8])
}

func shortAddress(pub []byte) string {
	value := hex.EncodeToString(pub)
	if len(value) <= 14 {
		return value
	}
	return value[:8] + "..." + value[len(value)-6:]
}

func readStatusField(store *state.Store, key []byte) string {
	raw, ok := store.Get(key)
	if !ok {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	v, _ := data["status"].(string)
	return v
}

func readNestedStringField(store *state.Store, key []byte, first, second string) string {
	raw, ok := store.Get(key)
	if !ok {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	node, ok := data[first].(map[string]any)
	if !ok {
		return ""
	}
	v, _ := node[second].(string)
	return v
}

func knownStatePrefixes() []string {
	return []string{
		state.PrefixAMXListing,
		state.PrefixAMXOrder,
		state.PrefixAMXIndex,
		state.PrefixAAPTask,
		state.PrefixAAPEvidence,
		state.PrefixAAPSLA,
		state.PrefixAAPHeartbeat,
		state.PrefixWeaveDAG,
		state.PrefixWeaveNode,
		state.PrefixCapUTXO,
		state.PrefixCapHolder,
		state.PrefixCapType,
		state.PrefixCapDerive,
		state.PrefixRepScore,
		state.PrefixArbDispute,
		state.PrefixArbVote,
		state.PrefixFiatEscrow,
		state.PrefixFiatDeposit,
		state.PrefixFiatPool,
		state.PrefixNodeReg,
		state.PrefixNodeVal,
		state.PrefixGovProp,
		state.PrefixGovVote,
		state.PrefixAccNonce,
		state.PrefixParams,
	}
}

func buildCapabilitiesDoc() capabilitiesResp {
	return capabilitiesResp{
		Project: "AACP v0.9.0",
		Purpose: "AACP 是一个面向 Agent 协作场景的链上执行骨架：负责交易验签、nonce/gas 管理、模块路由执行和可验证状态提交。",
		QuickStart: []string{
			"1) 先看“节点状态”，确认 status=ok。",
			"2) 点击“出一个空块”，观察 height 增长。",
			"3) 点击“CapUTXO 一键演示”，跑通 mint/delegate/revoke。",
			"4) 在“状态查询（按前缀）”输入 cap/u/，查看状态变化。",
			"5) 使用“发送自定义交易（Hex）”提交你自己的交易。",
		},
		WhatYouCanDo: []string{
			"发布与撮合任务/供需（AMX）",
			"创建任务、签 SLA、提交心跳与结果（AAP）",
			"管理可委托能力 UTXO（CapUTXO）",
			"资金托管、入金、释放、退款（FIAT）",
			"发起争议、投票仲裁、执行裁决（ARB）",
			"信誉评分更新和衰减（REP）",
			"节点注册、轮换、治理提案投票（NODE/GOV）",
		},
		StatePrefixes: knownStatePrefixes(),
		Modules: []capabilityModule{
			{
				ID:          "amx",
				Name:        "AMX 市场撮合",
				Description: "管理 listing / order / match 生命周期。",
				KeyPrefixes: []string{state.PrefixAMXListing, state.PrefixAMXOrder, state.PrefixAMXIndex},
				Actions: []capabilityAction{
					{Name: "create_listing", Description: "创建供给侧能力 listing"},
					{Name: "update_listing", Description: "更新 listing 参数"},
					{Name: "pause_listing", Description: "暂停 listing"},
					{Name: "delist_listing", Description: "下架 listing"},
					{Name: "submit_request", Description: "提交需求请求并生成订单"},
					{Name: "confirm_match", Description: "确认撮合"},
					{Name: "reject_match", Description: "拒绝撮合"},
					{Name: "cancel_order", Description: "取消订单"},
				},
			},
			{
				ID:          "aap",
				Name:        "AAP 任务协议",
				Description: "任务从创建到结算的执行主流程。",
				KeyPrefixes: []string{state.PrefixAAPTask, state.PrefixAAPEvidence, state.PrefixAAPSLA, state.PrefixAAPHeartbeat},
				Actions: []capabilityAction{
					{Name: "create_task", Description: "创建任务"},
					{Name: "sign_sla", Description: "签署 SLA"},
					{Name: "submit_heartbeat", Description: "提交心跳进度"},
					{Name: "submit_result", Description: "提交任务结果"},
					{Name: "verify_result", Description: "校验结果"},
					{Name: "settle_task", Description: "结算任务"},
					{Name: "file_dispute", Description: "发起争议"},
				},
			},
			{
				ID:          "weave",
				Name:        "WEAVE DAG 编排",
				Description: "管理 DAG 与节点执行状态。",
				KeyPrefixes: []string{state.PrefixWeaveDAG, state.PrefixWeaveNode},
				Actions: []capabilityAction{
					{Name: "create_dag", Description: "创建 DAG"},
					{Name: "start_dag", Description: "启动 DAG"},
					{Name: "update_node_status", Description: "更新节点状态"},
					{Name: "cancel_dag", Description: "取消 DAG"},
					{Name: "submit_node_output", Description: "提交节点输出"},
				},
			},
			{
				ID:          "caputxo",
				Name:        "CapUTXO 能力凭证",
				Description: "能力的 mint / spend / delegate / revoke 生命周期。",
				KeyPrefixes: []string{state.PrefixCapUTXO, state.PrefixCapHolder, state.PrefixCapType, state.PrefixCapDerive},
				Actions: []capabilityAction{
					{Name: "mint_capability", Description: "发行能力凭证"},
					{Name: "spend_capability", Description: "消耗能力额度"},
					{Name: "delegate_capability", Description: "委托子能力"},
					{Name: "revoke_capability", Description: "撤销能力并级联子能力"},
					{Name: "batch_expire", Description: "批量过期处理"},
				},
			},
			{
				ID:          "fiat",
				Name:        "FIAT 托管结算",
				Description: "法币托管与资金流转。",
				KeyPrefixes: []string{state.PrefixFiatEscrow, state.PrefixFiatDeposit, state.PrefixFiatPool},
				Actions: []capabilityAction{
					{Name: "create_escrow", Description: "创建托管单"},
					{Name: "fund_escrow", Description: "向托管入金"},
					{Name: "release_escrow", Description: "释放托管"},
					{Name: "lock_escrow", Description: "锁定托管"},
					{Name: "refund_escrow", Description: "退款"},
					{Name: "slash_deposit", Description: "罚没保证金"},
					{Name: "deposit_funds", Description: "充值资金"},
					{Name: "withdraw_deposit", Description: "提取充值"},
				},
			},
			{
				ID:          "arb",
				Name:        "ARB 仲裁",
				Description: "争议处理与投票裁决。",
				KeyPrefixes: []string{state.PrefixArbDispute, state.PrefixArbVote},
				Actions: []capabilityAction{
					{Name: "file_dispute", Description: "发起争议"},
					{Name: "submit_evidence", Description: "提交证据"},
					{Name: "auto_resolve", Description: "自动裁决"},
					{Name: "committee_vote", Description: "委员会投票"},
					{Name: "appeal", Description: "申诉"},
					{Name: "full_vote", Description: "全体投票"},
					{Name: "execute_verdict", Description: "执行裁决"},
				},
			},
			{
				ID:          "rep",
				Name:        "REP 信誉",
				Description: "信誉分维护与修正。",
				KeyPrefixes: []string{state.PrefixRepScore},
				Actions: []capabilityAction{
					{Name: "update_reputation", Description: "更新信誉分"},
					{Name: "apply_decay", Description: "应用衰减"},
					{Name: "override_score", Description: "人工覆盖分值"},
				},
			},
			{
				ID:          "node",
				Name:        "NODE 节点管理",
				Description: "节点注册与验证者管理。",
				KeyPrefixes: []string{state.PrefixNodeReg, state.PrefixNodeVal},
				Actions: []capabilityAction{
					{Name: "register_node", Description: "注册节点"},
					{Name: "rotate_key", Description: "轮换密钥"},
					{Name: "elect_validators", Description: "选举验证者"},
					{Name: "ban_node", Description: "封禁节点"},
				},
			},
			{
				ID:          "gov",
				Name:        "GOV 治理",
				Description: "提案创建、投票与执行。",
				KeyPrefixes: []string{state.PrefixGovProp, state.PrefixGovVote},
				Actions: []capabilityAction{
					{Name: "create_proposal", Description: "创建提案"},
					{Name: "vote_proposal", Description: "提案投票"},
					{Name: "execute_proposal", Description: "执行提案"},
				},
			},
			{
				ID:          "afd",
				Name:        "AFD 告警",
				Description: "上报告警事件并供后续策略处理。",
				KeyPrefixes: []string{},
				Actions: []capabilityAction{
					{Name: "report_alert", Description: "上报告警"},
				},
			},
		},
	}
}
