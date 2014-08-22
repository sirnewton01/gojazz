package main

import (
	"math/rand"
	"time"
)

const encodeURL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func generateUUID() string {
	src := rand.NewSource(time.Now().Unix())
	r := rand.New(src)

	uuid := make([]byte, 16)
	buffer := make([]byte, 23)
	nodeAddress := make([]byte, 6)

	clockSequence := r.Intn(16384)
	uuid[8] = byte(((clockSequence >> 8) & 0x3F) | 0x80)
	uuid[9] = byte(clockSequence & 0xFF)

	for i := 0; i < 6; i++ {
		nodeAddress[i] = byte(r.Intn(256))
	}

	nodeAddress[0] |= 0x80

	for i := 0; i < 6; i++ {
		uuid[i+10] = nodeAddress[i]
	}

	// Adjustment between Unix epoch and September 15, 1582
	epoch_adjustment := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC).Unix() - time.Date(1582, time.September, 15, 0, 0, 0, 0, time.UTC).Unix()

	currentTime := time.Now().Unix() + epoch_adjustment
	currentTime *= 10000
	currentTime |= 0x1000000000000000

	// Place the time into the byte array in network byte order.
	//
	for i := 0; i < 4; i++ {
		// time_low
		//
		uuid[i] = byte(currentTime >> uint(8*(3-i)) & 0xFF)
	}

	for i := 0; i < 2; i++ {
		// time_mid
		//
		uuid[i+4] = byte(currentTime >> uint(8*(1-i)+32) & 0xFF)
	}

	for i := 0; i < 2; i++ {
		// time_hi
		//
		uuid[i+6] = byte(currentTime >> uint(8*(1-i)+48) & 0xFF)
	}

	buffer[0] = '_'

	for i := 0; i < 5; i++ {
		buffer[4*i+1] = encodeURL[(uuid[i*3]>>2)&0x3F]
		buffer[4*i+2] = encodeURL[((uuid[i*3]<<4)&0x30)|((uuid[i*3+1]>>4)&0xF)]
		buffer[4*i+3] = encodeURL[((uuid[i*3+1]<<2)&0x3C)|((uuid[i*3+2]>>6)&0x3)]
		buffer[4*i+4] = encodeURL[uuid[i*3+2]&0x3F]
	}

	// Handle the last byte at the end.
	//
	buffer[21] = encodeURL[(uuid[15]>>2)&0x3F]
	buffer[22] = encodeURL[(uuid[15]<<4)&0x30]

	return string(buffer)
}
