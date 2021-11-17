package quorum

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/MixinNetwork/mixin/common"
	"github.com/MixinNetwork/mixin/domains/ethereum"
	"github.com/MixinNetwork/trusted-group/mvm/encoding"
	"github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	ClockTick = 3 * time.Second
	// event MixinTransaction(bytes);
	EventTopic = "0xdb53e751d28ed0d6e3682814bf8d23f7dd7b29c94f74a56fbb7f88e9dca9f39b"
	// function mixin(bytes calldata raw) public returns (bool)
	EventMethod = "0x5cae8005"

	GasLimit = 100000000
	GasPrice = 10000
)

type Configuration struct {
	Store      string `toml:"store"`
	RPC        string `toml:"rpc"`
	PrivateKey string `toml:"key"`
}

type Engine struct {
	db  *badger.DB
	rpc *RPC
	key string
}

func Boot(conf *Configuration) (*Engine, error) {
	db := openBadger(conf.Store)
	rpc, err := NewRPC(conf.RPC)
	if err != nil {
		return nil, err
	}
	e := &Engine{db: db, rpc: rpc}
	if conf.PrivateKey != "" {
		priv, err := crypto.HexToECDSA(conf.PrivateKey)
		if err != nil {
			panic(err)
		}
		e.key = hex.EncodeToString(crypto.FromECDSA(priv))
	}
	return e, nil
}

func (e *Engine) Hash(b []byte) []byte {
	return crypto.Keccak256(b)
}

func (e *Engine) VerifyAddress(address string, hash []byte) error {
	err := ethereum.VerifyAddress(address)
	if err != nil {
		return err
	}
	height, err := e.rpc.GetBlockHeight()
	if err != nil {
		panic(err)
	}
	birth, err := e.rpc.GetContractBirthBlock(address, string(hash))
	if err != nil && strings.Contains(err.Error(), "malformed") {
		return err
	} else if err != nil {
		panic(err)
	}
	if height < birth+128 {
		return fmt.Errorf("too young %d %d", birth, height)
	}
	// TODO ABI
	e.storeWriteContractLogsOffset(address, birth)
	return nil
}

func (e *Engine) SetupNotifier(address string) error {
	// seed = hash(e.key + address)
	// key from seed
	// read contract notifier state
	key := ""
	return e.storeWriteContractNotifier(address, key, "initial")
}

func (e *Engine) EstimateCost(events []*encoding.Event) (common.Integer, error) {
	// TODO should do it
	return common.Zero, nil
}

func (e *Engine) EnsureSendGroupEvents(address string, events []*encoding.Event) error {
	return e.storeWriteGroupEvents(address, events)
}

func (e *Engine) ReceiveGroupEvents(address string, offset uint64, limit int) ([]*encoding.Event, error) {
	return e.storeListContractEvents(address, offset, limit)
}

func (e *Engine) loopGetLogs(address string) {
	nonce := e.storeReadLastContractEventNonce(address) + 1
	for {
		offset := e.storeReadContractLogsOffset(address)
		logs, err := e.rpc.GetLogs(address, EventTopic, offset, offset+10)
		if err != nil {
			panic(err)
		}
		var evts []*encoding.Event
		for _, b := range logs {
			evt, err := encoding.DecodeEvent(b)
			if err != nil {
				panic(err)
			}
			evts = append(evts, evt)
		}
		sort.Slice(evts, func(i, j int) bool { return evts[i].Nonce < evts[j].Nonce })
		for _, evt := range evts {
			if evt.Nonce < nonce {
				continue
			}
			if evt.Nonce > nonce {
				break
			}
			e.storeWriteContractEvent(address, evt)
			nonce = nonce + 1
		}
		e.storeWriteContractLogsOffset(address, offset+10)
		if len(logs) == 0 {
			time.Sleep(ClockTick * 5)
		}
	}
}

func (e *Engine) loopSendGroupEvents(address string) {
	notifier := e.storeReadContractNotifier(address)
	for e.key != "" {
		nonce, err := e.rpc.GetAddressNonce(address)
		if err != nil {
			panic(err)
		}
		evts, err := e.storeListGroupEvents(address, nonce, 100)
		if err != nil {
			panic(err)
		}
		for _, evt := range evts {
			raw := e.signGroupEventTransaction(address, evt, notifier)
			_, err := e.rpc.SendRawTransaction(raw)
			if err != nil {
				panic(err)
			}
		}
		if len(evts) == 0 {
			time.Sleep(ClockTick)
		}
	}
}

func (e *Engine) loopHandleContracts() {
	for {
		// read all contracts
		// see if notifier setup => setup
		// see if they are running
		// then loopGetLogs
		// then loopSendGroupEvents
	}
}
