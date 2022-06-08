package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"log"
	"os"
	"strconv"

	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/niftynei/glightning/glightning"
)

func main() {
	plugin := glightning.NewPlugin(onInit)
	plugin.RegisterHooks(&glightning.Hooks{
		HtlcAccepted: OnHtlcAccepted,
	})

	err := plugin.Start(os.Stdin, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}

func onInit(plugin *glightning.Plugin, options map[string]glightning.Option, config *glightning.Config) {
	log.Printf("successfully init'd! %s\n", config.RpcFile)
}

func OnHtlcAccepted(event *glightning.HtlcAcceptedEvent) (*glightning.HtlcAcceptedResponse, error) {
	log.Printf("htlc_accepted called\n")

	onion := event.Onion

	milliSatoshiOutAmount := onion.ForwardAmount[:len(onion.ForwardAmount)-4]
	OutgoingAmountMsat, _ := strconv.Atoi(milliSatoshiOutAmount)

	milliSatoshiTotalAmount := onion.TotalMilliSatoshi[:len(onion.TotalMilliSatoshi)-4]
	incomingAmountMsat, err := strconv.Atoi(milliSatoshiTotalAmount)

	//var bigProd, bigAmt big.Int
	//amt := OutgoingAmountMsat + 10
	//amt := (bigAmt.Div(bigProd.Mul(big.NewInt(int64(OutgoingAmountMsat)), big.NewInt(int64(OutgoingAmountMsat))), big.NewInt(int64(incomingAmountMsat)))).Int64()
	//fAmt := strconv.FormatInt(amt, 10)

	var addr [32]byte
	paysec, _ := hex.DecodeString(onion.PaymentSecret)
	copy(addr[:], paysec)
	on := glightning.Onion{
		Payload:           onion.Payload,
		NextOnion:         onion.NextOnion,
		SharedSecret:      onion.SharedSecret,
		PerHop:            onion.PerHop,
		Type:              onion.Type,
		ShortChannelId:    onion.ShortChannelId,
		OutgoingCltv:      onion.OutgoingCltv,
		ForwardAmount:     onion.ForwardAmount, //fmt.Sprint(amt) + "msat",
		PaymentSecret:     onion.PaymentSecret,
		TotalMilliSatoshi: onion.TotalMilliSatoshi,
	}

	//payload, err := json.Marshal(on)
	//if err != nil {
	//	log.Printf("json marshal error : %v", err)
	//}
	//log.Printf("json marshal payload : %v", payload)
	//pyld := hex.EncodeToString(on.Payload)
	//log.Printf("EncodeToString payload : %v", on.Payload)
	/*
		hop := onion.PerHop{
			ForwardAmountMilliSatoshis: amt,
			OutgoingCltvValue:          onion.OutgoingCltv,
			ShortChannelId:             onion.ShortChannelId,
			Realm:                      " ",
			//MPP:                      record.NewMPP(lnwire.MilliSatoshi(outgoingAmountMsat), addr),
		}
	*/
	/*
		var b bytes.Buffer
		err = PackHopPayload(&b, uint64(0))
		log.Printf("hop.PackHopPayload(): %v", err)

		payload, err := sphinx.NewHopPayload(nil, b.Bytes())
		log.Printf("sphinx.NewHopPayload(): %v", err)
	*/
	/*
		payload, err := json.Marshal(on)
		if err != nil {
			log.Printf("%v", err)
		}
	*/
	/*
		var b bytes.Buffer
		err = hop.PackHopPayload(&b, uint64(0))
		log.Printf("hop.PackHopPayload(): %v", err)

		payload, err := sphinx.NewHopPayload(nil, b.Bytes())
		log.Printf("sphinx.NewHopPayload(): %v", err)
	*/

	var b bytes.Buffer
	//enc := gob.NewEncoder(&b)
	//err = enc.Encode(on)
	//if err != nil {
	//	log.Fatal("encode error:", err)
	//}

	var records []tlv.Record
	amtf := uint64(OutgoingAmountMsat + 1)
	//amtf, _ := strconv.ParseUint(milliSatoshiOutAmount, 10, 64)
	log.Printf("amount forward -----> %v", amtf)
	ocltv := uint32(on.OutgoingCltv)
	records = append(records,
		record.NewAmtToFwdRecord(&amtf),
		record.NewLockTimeRecord(&ocltv),
	)
	MPP := record.NewMPP(lnwire.MilliSatoshi(int64(incomingAmountMsat+1)), addr)
	nextChanID := uint64(0)
	if MPP != nil {
		if nextChanID == 0 {
			records = append(records, MPP.Record())
		} else {
			log.Printf(" nextChanID next channel id not zero ")
		}
	}
	/*
		scId, _ := strconv.Atoi(on.ShortChannelId)
		UscId := uint64(scId)
		if scId != 0 {
			records = append(records,
				record.NewNextHopIDRecord(&UscId),
			)
		}
	*/
	tlv.SortRecords(records)
	tlvStream, err := tlv.NewStream(records...)
	if err != nil {
		log.Printf("tlvStream error : %v", err)
	}
	var w io.Writer
	w = &b
	err = tlvStream.Encode(w)
	if err != nil {
		log.Printf("tlvStream.Encode(w) : %v", err)
	}
	log.Printf("tlvStream bytes  : %v", b)

	payload, err := sphinx.NewHopPayload(nil, b.Bytes())
	log.Printf("sphinx.NewHopPayload(): %v", err)

	/*
		_, _, destination, _, _, _, _, err := paymentInfo([]byte(event.Htlc.PaymentHash))
		if err != nil {
			log.Printf("paymentInfo(%x) error: %v", event.Htlc.PaymentHash, err)
		}
		log.Printf("\ndestination:%x\n\n\n",
			destination)
	*/

	/*
		pkBytes, err := hex.DecodeString("0215df18e0653f1734bf1319bee09512edb93da3808aa05e0ae203b51096a9dc47")
		pubKey, err := btcec.ParsePubKey(pkBytes)
		log.Printf("btcec.ParsePubKey(%x): %v", onion.ShortChannelId, err)

		var sphinxPath sphinx.PaymentPath
		sphinxPath[0] = sphinx.OnionHop{
			NodePub:    *pubKey,
			HopPayload: payload,
		}
		sessionKey, err := btcec.NewPrivateKey()
		log.Printf("btcec.NewPrivateKey(): %v", err)
		sphinxPacket, err := sphinx.NewOnionPacket(
			&sphinxPath, sessionKey, []byte(event.Htlc.PaymentHash),
			sphinx.DeterministicPacketFiller,
		)
		log.Printf("sphinxPacket %v", err)
		var onionBlob bytes.Buffer
		err = sphinxPacket.Encode(&onionBlob)
		log.Printf("sphinxPacket.Encode(): %v", err)
	*/
	// hex encode of stream bytes
	//return event.ContinueWithPayload(hex.EncodeToString(onionBlob.Bytes())), nil
	return event.ContinueWithPayload(hex.EncodeToString(payload.Payload)), nil
	//pl := tlv.Marshal(event.Onion.Payload)
	//return event.ContinueWithPayload(pl), nil
}
