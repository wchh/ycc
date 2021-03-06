// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

//database opeartion for execs ticket
import (

	//"bytes"

	"fmt"

	"github.com/33cn/chain33/account"
	"github.com/33cn/chain33/client"
	"github.com/33cn/chain33/common/address"
	dbm "github.com/33cn/chain33/common/db"
	log "github.com/33cn/chain33/common/log/log15"
	"github.com/33cn/chain33/system/dapp"
	"github.com/33cn/chain33/types"
	ty "github.com/yccproject/ycc/plugin/dapp/pos33/types"
)

var tlog = log.New("module", "ticket.db")

//var genesisKey = []byte("mavl-acc-genesis")
//var addrSeed = []byte("address seed bytes for public key")

// DB db
type DB struct {
	ty.Pos33Ticket
	prevstatus int32
}

//GetRealPrice 获取真实的价格
func (t *DB) GetRealPrice(cfg *types.Chain33Config) int64 {
	if t.GetPrice() == 0 {
		cfg := ty.GetPos33TicketMinerParam(cfg, cfg.GetFork("ForkChainParamV1"))
		return cfg.Pos33TicketPrice
	}
	return t.GetPrice()
}

// NewDB new instance
func NewDB(cfg *types.Chain33Config, id, minerAddress, returnWallet string, blocktime, height, price int64, isGenesis bool) *DB {
	t := &DB{}
	t.TicketId = id
	t.MinerAddress = minerAddress
	t.ReturnAddress = returnWallet
	t.CreateTime = blocktime
	t.Status = 1
	t.IsGenesis = isGenesis
	t.prevstatus = 0
	//height == 0 的情况下，不去改变 genesis block
	if cfg.IsFork(height, "ForkChainParamV2") && height > 0 {
		t.Price = price
	}
	return t
}

//ticket 的状态变化：
//1. status == 1 (NewPos33Ticket的情况)
//2. status == 2 (已经挖矿的情况)
//3. status == 3 (Close的情况)

//add prevStatus:  便于回退状态，以及删除原来状态
//list 保存的方法:
//minerAddress:status:ticketId=ticketId

// GetReceiptLog get receipt
func (t *DB) GetReceiptLog(typ int32) *types.ReceiptLog {
	log := &types.ReceiptLog{}
	/*
		if t.Status == 1 {
			log.Ty = ty.TyLogNewPos33Ticket
		} else if t.Status == 2 {
			log.Ty = ty.TyLogMinerPos33Ticket
		} else if t.Status == 3 {
			log.Ty = ty.TyLogClosePos33Ticket
		}
	*/
	log.Ty = typ
	r := &ty.ReceiptPos33Ticket{}
	r.TicketId = t.TicketId
	r.Status = t.Status
	r.PrevStatus = t.prevstatus
	r.Addr = t.MinerAddress
	log.Log = types.Encode(r)
	return log
}

// GetKVSet get kv set
func (t *DB) GetKVSet() (kvset []*types.KeyValue) {
	value := types.Encode(&t.Pos33Ticket)
	kvset = append(kvset, &types.KeyValue{Key: Key(t.TicketId), Value: value})
	return kvset
}

// Save save
func (t *DB) Save(db dbm.KV) {
	set := t.GetKVSet()
	for i := 0; i < len(set); i++ {
		db.Set(set[i].GetKey(), set[i].Value)
	}
}

//Key address to save key
func Key(id string) (key []byte) {
	key = append(key, []byte("mavl-pos33-")...)
	key = append(key, []byte(id)...)
	return key
}

// BindKey bind key
func BindKey(id string) (key []byte) {
	key = append(key, []byte("mavl-pos33-tbind-")...)
	key = append(key, []byte(id)...)
	return key
}

// Action action type
type Action struct {
	coinsAccount *account.DB
	db           dbm.KV
	txhash       []byte
	fromaddr     string
	blocktime    int64
	height       int64
	execaddr     string
	api          client.QueueProtocolAPI
}

// NewAction new action type
func NewAction(t *Pos33Ticket, tx *types.Transaction) *Action {
	hash := tx.Hash()
	fromaddr := tx.From()
	return &Action{t.GetCoinsAccount(), t.GetStateDB(), hash, fromaddr,
		t.GetBlockTime(), t.GetHeight(), dapp.ExecAddress(string(tx.Execer)), t.GetAPI()}
}

// GenesisInit init genesis
func (action *Action) GenesisInit(genesis *ty.Pos33TicketGenesis) (*types.Receipt, error) {
	chain33Cfg := action.api.GetConfig()
	prefix := fmt.Sprintf("%s:%d:", genesis.MinerAddress[:8], action.height)
	var logs []*types.ReceiptLog
	var kv []*types.KeyValue
	cfg := ty.GetPos33TicketMinerParam(chain33Cfg, action.height)
	for i := 0; i < int(genesis.Count); i++ {
		id := prefix + fmt.Sprintf("%010d", i)
		t := NewDB(chain33Cfg, id, genesis.MinerAddress, genesis.ReturnAddress, action.blocktime, action.height, cfg.Pos33TicketPrice, true)
		//冻结子账户资金
		receipt, err := action.coinsAccount.ExecFrozen(genesis.ReturnAddress, action.execaddr, cfg.Pos33TicketPrice)
		if err != nil {
			tlog.Error("GenesisInit.Frozen", "addr", genesis.ReturnAddress, "execaddr", action.execaddr)
			panic(err)
		}
		t.Save(action.db)
		logs = append(logs, t.GetReceiptLog(ty.TyLogNewPos33Ticket))
		kv = append(kv, t.GetKVSet()...)
		logs = append(logs, receipt.Logs...)
		kv = append(kv, receipt.KV...)
	}
	receipt := &types.Receipt{Ty: types.ExecOk, KV: kv, Logs: logs}
	return receipt, nil
}

func saveBind(db dbm.KV, tbind *ty.Pos33TicketBind) {
	set := getBindKV(tbind)
	for i := 0; i < len(set); i++ {
		db.Set(set[i].GetKey(), set[i].Value)
	}
}

func getBindKV(tbind *ty.Pos33TicketBind) (kvset []*types.KeyValue) {
	value := types.Encode(tbind)
	kvset = append(kvset, &types.KeyValue{Key: BindKey(tbind.ReturnAddress), Value: value})
	return kvset
}

func getBindLog(tbind *ty.Pos33TicketBind, old string) *types.ReceiptLog {
	log := &types.ReceiptLog{}
	log.Ty = ty.TyLogPos33TicketBind
	r := &ty.ReceiptPos33TicketBind{}
	r.ReturnAddress = tbind.ReturnAddress
	r.OldMinerAddress = old
	r.NewMinerAddress = tbind.MinerAddress
	log.Log = types.Encode(r)
	return log
}

func (action *Action) getBind(addr string) string {
	value, err := action.db.Get(BindKey(addr))
	if err != nil || value == nil {
		return ""
	}
	var bind ty.Pos33TicketBind
	err = types.Decode(value, &bind)
	if err != nil {
		panic(err)
	}
	return bind.MinerAddress
}

//Pos33TicketBind 授权某个地址进行挖矿
func (action *Action) Pos33TicketBind(tbind *ty.Pos33TicketBind) (*types.Receipt, error) {
	//todo: query address is a minered address
	if action.fromaddr != tbind.ReturnAddress {
		return nil, types.ErrFromAddr
	}
	//"" 表示设置为空
	if len(tbind.MinerAddress) > 0 {
		if err := address.CheckAddress(tbind.MinerAddress); err != nil {
			return nil, err
		}
	}
	var logs []*types.ReceiptLog
	var kvs []*types.KeyValue
	oldbind := action.getBind(tbind.ReturnAddress)
	log := getBindLog(tbind, oldbind)
	logs = append(logs, log)
	saveBind(action.db, tbind)
	kv := getBindKV(tbind)
	kvs = append(kvs, kv...)
	receipt := &types.Receipt{Ty: types.ExecOk, KV: kvs, Logs: logs}
	return receipt, nil
}

// Pos33TicketOpen ticket open
func (action *Action) Pos33TicketOpen(topen *ty.Pos33TicketOpen) (*types.Receipt, error) {
	chain33Cfg := action.api.GetConfig()
	prefix := fmt.Sprintf("%s:%d:", topen.MinerAddress[:8], action.height)
	var logs []*types.ReceiptLog
	var kv []*types.KeyValue
	//addr from
	if action.fromaddr != topen.ReturnAddress {
		mineraddr := action.getBind(topen.ReturnAddress)
		if mineraddr != action.fromaddr {
			return nil, ty.ErrMinerNotPermit
		}
		if topen.MinerAddress != mineraddr {
			return nil, ty.ErrMinerAddr
		}
	}
	//action.fromaddr == topen.ReturnAddress or mineraddr == action.fromaddr
	cfg := ty.GetPos33TicketMinerParam(chain33Cfg, action.height)
	for i := 0; i < int(topen.Count); i++ {
		id := prefix + fmt.Sprintf("%010d", i)
		/*
			//add pubHash
			if chain33Cfg.IsDappFork(action.height, ty.Pos33TicketX, "ForkTicketId") {
				if len(topen.PubHashes) == 0 {
					return nil, ty.ErrOpenPos33TicketPubHash
				}
				id = id + ":" + fmt.Sprintf("%x:%d", topen.PubHashes[i], topen.RandSeed)
			}
		*/
		t := NewDB(chain33Cfg, id, topen.MinerAddress, topen.ReturnAddress, action.blocktime, action.height, cfg.Pos33TicketPrice, false)

		//冻结子账户资金
		receipt, err := action.coinsAccount.ExecFrozen(topen.ReturnAddress, action.execaddr, cfg.Pos33TicketPrice)
		if err != nil {
			tlog.Error("Pos33TicketOpen.Frozen", "addr", topen.ReturnAddress, "execaddr", action.execaddr, "n", topen.Count)
			return nil, err
		}
		t.Save(action.db)
		logs = append(logs, t.GetReceiptLog(ty.TyLogNewPos33Ticket))
		kv = append(kv, t.GetKVSet()...)
		logs = append(logs, receipt.Logs...)
		kv = append(kv, receipt.KV...)
	}
	tlog.Info("@@@@@@@ pos33.ticket open", "ntid", topen.Count, "height", action.height)
	receipt := &types.Receipt{Ty: types.ExecOk, KV: kv, Logs: logs}
	return receipt, nil
}

func readPos33Ticket(db dbm.KV, id string) (*ty.Pos33Ticket, error) {
	data, err := db.Get(Key(id))
	if err != nil {
		return nil, err
	}
	var ticket ty.Pos33Ticket
	//decode
	err = types.Decode(data, &ticket)
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// Pos33TicketMiner ticket miner
func (action *Action) Pos33TicketMiner(miner *ty.Pos33Miner, index int) (*types.Receipt, error) {
	if index != 0 {
		return nil, types.ErrCoinBaseIndex
	}
	chain33Cfg := action.api.GetConfig()
	sumw := len(miner.GetVotes())

	var kvs []*types.KeyValue
	var logs []*types.ReceiptLog

	// reward voters
	for _, v := range miner.Votes {
		r := v.Sort
		tid := r.SortHash.Tid
		t, err := readPos33Ticket(action.db, tid)
		if err != nil {
			return nil, err
		}

		receipt, err := action.coinsAccount.ExecDepositFrozen(t.ReturnAddress, action.execaddr, ty.Pos33VoteReward)
		if err != nil {
			tlog.Error("Pos33TicketMiner.ExecDepositFrozen error", "voter", t.ReturnAddress, "execaddr", action.execaddr)
			return nil, err
		}

		kvs = append(kvs, receipt.GetKV()...)
		logs = append(logs, receipt.GetLogs()...)

		t.MinerValue += ty.Pos33VoteReward
		prevStatus := t.Status
		t.Status = 1 // here, Don't change to 2,
		db := &DB{*t, prevStatus}
		db.Save(action.db)
		logs = append(logs, db.GetReceiptLog(ty.TyLogMinerPos33Ticket))
		kvs = append(kvs, db.GetKVSet()...)
	}

	// bp reward
	bpReward := ty.Pos33BpReward * int64(sumw)
	if bpReward > 0 {
		tid := miner.Sort.SortHash.Tid
		t, err := readPos33Ticket(action.db, tid)
		if err != nil {
			return nil, err
		}

		receipt1, err := action.coinsAccount.ExecDepositFrozen(t.ReturnAddress, action.execaddr, bpReward)
		if err != nil {
			tlog.Error("Pos33TicketMiner.ExecDepositFrozen error", "error", err, "bp", t.ReturnAddress, "value", bpReward)
			return nil, err
		}
		kvs = append(kvs, receipt1.GetKV()...)
		logs = append(logs, receipt1.GetLogs()...)

		tlog.Info("bp rerward", "height", action.height, "tid", t.TicketId, "minerAddr", t.MinerAddress, "returnAddr", t.ReturnAddress, "reward", bpReward)
		t.MinerValue += bpReward
		prevStatus := t.Status
		t.Status = 1
		db := &DB{*t, prevStatus}
		db.Save(action.db)
		logs = append(logs, db.GetReceiptLog(ty.TyLogMinerPos33Ticket))
		kvs = append(kvs, db.GetKVSet()...)
	}

	// fund reward
	fundReward := ty.Pos33BlockReward - (ty.Pos33VoteReward+ty.Pos33BpReward)*int64(sumw)
	tlog.Info("fund rerward", "height", action.height, "reward", fundReward)
	if fundReward > 0 {
		var receipt2 *types.Receipt
		var err error
		// issue coins to exec addr
		addr := chain33Cfg.MGStr("mver.consensus.fundKeyAddr", action.height)
		receipt2, err = action.coinsAccount.ExecIssueCoins(addr, fundReward)
		if err != nil {
			tlog.Error("Pos33TicketMiner.ExecIssueCoins fund to autonomy fund", "addr", addr, "error", err)
			return nil, err
		}
		logs = append(logs, receipt2.Logs...)
		kvs = append(kvs, receipt2.KV...)
	}

	return &types.Receipt{Ty: types.ExecOk, KV: kvs, Logs: logs}, nil
}

// Pos33TicketClose close tick
func (action *Action) Pos33TicketClose(tclose *ty.Pos33TicketClose) (*types.Receipt, error) {
	chain33Cfg := action.api.GetConfig()
	tickets := make([]*DB, len(tclose.TicketId))
	cfg := ty.GetPos33TicketMinerParam(chain33Cfg, action.height)
	for i := 0; i < len(tclose.TicketId); i++ {
		ticket, err := readPos33Ticket(action.db, tclose.TicketId[i])
		if err != nil {
			return nil, err
		}
		//ticket 的生成时间超过 2天,可提款
		if ticket.Status != 1 {
			tlog.Error("ticket", "id", ticket.GetTicketId(), "status", ticket.GetStatus())
			return nil, ty.ErrPos33TicketClosed
		}
		if !ticket.IsGenesis {
			if action.blocktime-ticket.GetCreateTime() < cfg.Pos33TicketWithdrawTime {
				return nil, ty.ErrTime
			}
		}
		//check from address
		if action.fromaddr != ticket.MinerAddress && action.fromaddr != ticket.ReturnAddress {
			return nil, types.ErrFromAddr
		}
		prevstatus := ticket.Status
		ticket.Status = 3
		tickets[i] = &DB{*ticket, prevstatus}
	}
	var logs []*types.ReceiptLog
	var kv []*types.KeyValue
	for i := 0; i < len(tickets); i++ {
		t := tickets[i]
		retValue := t.GetRealPrice(chain33Cfg) + t.MinerValue
		receipt1, err := action.coinsAccount.ExecActive(t.ReturnAddress, action.execaddr, retValue)
		if err != nil {
			tlog.Error("Pos33TicketClose.ExecActive user", "addr", t.ReturnAddress, "execaddr", action.execaddr, "value", retValue)
			return nil, err
		}
		tlog.Info("close pos33.ticket", "tid", t.TicketId, "height", action.height, "activeValue", retValue)
		logs = append(logs, t.GetReceiptLog(ty.TyLogClosePos33Ticket))
		kv = append(kv, t.GetKVSet()...)
		logs = append(logs, receipt1.Logs...)
		kv = append(kv, receipt1.KV...)
		t.Save(action.db)
	}
	tlog.Info("@@@@@@@ pos33.ticket close", "ntid", len(tclose.TicketId), "height", action.height)
	receipt := &types.Receipt{Ty: types.ExecOk, KV: kv, Logs: logs}
	return receipt, nil
}

// List list db
func List(db dbm.Lister, db2 dbm.KV, tlist *ty.Pos33TicketList) (types.Message, error) {
	values, err := db.List(calcPos33TicketPrefix(tlist.Addr, tlist.Status), nil, 0, 0)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return &ty.ReplyPos33TicketList{}, nil
	}
	var ids ty.Pos33TicketInfos
	for i := 0; i < len(values); i++ {
		ids.TicketIds = append(ids.TicketIds, string(values[i]))
	}
	return Infos(db2, &ids)
}

// Infos info
func Infos(db dbm.KV, tinfos *ty.Pos33TicketInfos) (types.Message, error) {
	var tickets []*ty.Pos33Ticket
	for i := 0; i < len(tinfos.TicketIds); i++ {
		id := tinfos.TicketIds[i]
		ticket, err := readPos33Ticket(db, id)
		//数据库可能会不一致，读的过程中可能会有写
		if err != nil {
			continue
		}
		tickets = append(tickets, ticket)
	}
	return &ty.ReplyPos33TicketList{Tickets: tickets}, nil
}
