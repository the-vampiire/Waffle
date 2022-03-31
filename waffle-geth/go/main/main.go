package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"strings"
	"time"

	"github.com/Ethworks/Waffle/simulator"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

/*
// C code written here would be compiled together with Go code.

#include "main.h"

*/
import "C"

type TransactionRequest = C.TransactionRequest

func main() {}

var (
	simulators      = make(map[int]*simulator.Simulator)
	nextSimulatorID = 0
)

//export newSimulator
func newSimulator() C.int {
	sim, err := simulator.NewSimulator()
	if err != nil {
		log.Fatal(err)
	}
	id := nextSimulatorID
	simulators[id] = sim
	nextSimulatorID++
	return C.int(id)
}

//export getBlockNumber
func getBlockNumber(simID C.int) *C.char {
	sim := getSimulator(simID)
	bn := sim.GetLatestBlockNumber()
	// TODO: Convert to base 16?
	return C.CString(bn.String())
}

//export getCode
func getCode(simID C.int, account *C.char) *C.char {
	sim := getSimulator(simID)
	code, err := sim.Backend.CodeAt(context.Background(), common.HexToAddress(C.GoString(account)), nil)
	if err != nil {
		log.Fatal(err)
	}

	return C.CString(common.Bytes2Hex(code))
}

//export getChainID
func getChainID(simID C.int) *C.char {
	sim := getSimulator(simID)
	bn := sim.GetChainID()
	return C.CString(bn.String())
}

//export getBalance
func getBalance(simID C.int, account *C.char) *C.char {
	sim := getSimulator(simID)
	bal, err := sim.Backend.BalanceAt(context.Background(), common.HexToAddress(C.GoString(account)), nil)
	if err != nil {
		log.Fatal(err)
	}

	return C.CString(bal.String())
}

//export getTransactionCount
func getTransactionCount(simID C.int, account *C.char) C.int {
	sim := getSimulator(simID)
	count, err := sim.Backend.NonceAt(context.Background(), common.HexToAddress(C.GoString(account)), nil)
	if err != nil {
		log.Fatal(err)
	}

	return C.int(count)
}

//export getLogs
func getLogs(simID C.int, queryJson *C.char) *C.char {
	sim := getSimulator(simID)

	var query ethereum.FilterQuery

	err := json.Unmarshal([]byte(C.GoString(queryJson)), &query)
	if err != nil {
		log.Fatal(err)
	}

	logs, err := sim.Backend.FilterLogs(context.Background(), query)

	if err != nil {
		log.Fatal(err)
	}

	logsJson, err := json.Marshal(logs)
	return C.CString(string(logsJson))
}

//export call
func call(simID C.int, msg TransactionRequest) *C.char {
	sim := getSimulator(simID)

	var callMsg ethereum.CallMsg

	if msg.From != nil {
		callMsg.From = common.HexToAddress(C.GoString(msg.From))
	}
	if msg.To != nil {
		temp := common.HexToAddress(C.GoString(msg.To))
		callMsg.To = &temp
	}
	callMsg.Gas = uint64(msg.Gas)

	gasPrice := C.GoString(msg.GasPrice)
	if gasPrice != "" {
		callMsg.GasPrice = big.NewInt(0)
		callMsg.GasPrice.SetString(gasPrice, 16)
	}

	gasFeeCap := C.GoString(msg.GasFeeCap)
	if gasFeeCap != "" {
		callMsg.GasFeeCap = big.NewInt(0)
		callMsg.GasFeeCap.SetString(gasFeeCap, 16)
	}

	gasTipCap := C.GoString(msg.GasTipCap)
	if gasTipCap != "" {
		callMsg.GasTipCap = big.NewInt(0)
		callMsg.GasTipCap.SetString(gasTipCap, 16)
	}

	if msg.Data != nil {
		data, err := hex.DecodeString(C.GoString(msg.Data)[2:])
		if err != nil {
			log.Fatal(err)
		}

		callMsg.Data = data
	}

	res, err := sim.Backend.CallContract(context.Background(), callMsg, nil)
	if err != nil {
		log.Fatal(err)
	}

	return C.CString(hex.EncodeToString(res))
}

type txReceipt struct {
	Tx        *types.Transaction
	IsPending bool
}

//export getTransaction
func getTransaction(simID C.int, txHash *C.char) *C.char {
	sim := getSimulator(simID)

	hash := common.HexToHash(C.GoString(txHash))
	tx, isPending, err := sim.Backend.SimulatedBackend.TransactionByHash(context.Background(), hash)
	if err != nil {
		log.Fatal(err)
	}

	stringified, err := json.Marshal(txReceipt{Tx: tx, IsPending: isPending})
	if err != nil {
		log.Fatal(err)
	}

	return C.CString(string(stringified[:]))
}

type TransactionReceipt = C.TransactionReceipt

//export sendTransaction
func sendTransaction(simID C.int, txData *C.char) TransactionReceipt {
	sim := getSimulator(simID)

	bytes, err := hex.DecodeString(C.GoString(txData)[2:])
	if err != nil {
		log.Fatal(err)
	}

	tx := &types.Transaction{}
	err = tx.UnmarshalBinary(bytes)
	if err != nil {
		log.Fatal(err)
	}

	err = sim.Backend.SimulatedBackend.SendTransaction(context.Background(), tx)
	if err != nil {
		log.Fatal(err)
	}

	sim.Backend.Commit()

	receipt, err := sim.Backend.SimulatedBackend.TransactionReceipt(context.Background(), tx.Hash())
	if err != nil {
		log.Fatal(err)
	}

	return TransactionReceipt{
		Type:              C.uchar(receipt.Type),
		Status:            C.ulonglong(receipt.Status),
		CumulativeGasUsed: C.ulonglong(receipt.CumulativeGasUsed),
		TxHash:            C.CString(receipt.TxHash.String()),
		ContractAddress:   C.CString(receipt.ContractAddress.String()),
		GasUsed:           C.ulonglong(receipt.GasUsed),
		BlockHash:         C.CString(receipt.BlockHash.String()),
		BlockNumber:       C.CString(receipt.BlockNumber.Text(16)),
		TransactionIndex:  C.uint(receipt.TransactionIndex),
	}
}

func getSimulator(simID C.int) *simulator.Simulator {
	id := int(simID)
	sim, ok := simulators[id]
	if !ok {
		log.Fatal(fmt.Errorf("simulator with %d does not exist", id))
	}
	return sim
}

//export cgoCurrentMillis
func cgoCurrentMillis() C.long {
	return C.long(time.Now().Unix())
}

//export cgoSeed
func cgoSeed(m C.long) {
	rand.Seed(int64(m))
}

//export cgoRandom
func cgoRandom(m C.int) C.int {
	return C.int(rand.Intn(int(m)))
}

//export countLines
func countLines(str *C.char) int32 {
	go_str := C.GoString(str)
	n := strings.Count(go_str, "\n")
	return int32(n)
}

//export toUpper
func toUpper(str *C.char) *C.char {
	go_str := C.GoString(str)
	upper := strings.ToUpper(go_str)
	return C.CString(upper)
}

type InputStruct = C.InputStruct
type OutputStruct = C.OutputStruct

//export sumProduct
func sumProduct(intput InputStruct) OutputStruct {
	return OutputStruct{
		Sum:     intput.A + intput.B,
		Product: intput.A * intput.B,
	}
}
