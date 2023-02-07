package main

import (
	"image"
	"image/jpeg"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/format"
	"github.com/aler9/gortsplib/pkg/formatdecenc/rtph264"
	"github.com/aler9/gortsplib/pkg/url"
	"github.com/pion/rtp"
)

// This example shows how to
// 1. connect to a RTSP server
// 2. check if there's a H264 media
// 3. save the content of the H264 media into a file in MPEG-TS format

func main() {
	c := gortsplib.Client{}

	// parse URL
	u, err := url.Parse("rtsp://Ok7LGIbySdDk:Fw41BEBGSBMg@192.168.252.26/live0")
	if err != nil {
		panic(err)
	}

	// connect to the server
	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	// find published medias
	medias, baseURL, _, err := c.Describe(u)
	if err != nil {
		panic(err)
	}

	// find the H264 media and format
	var forma *format.H264
	medi := medias.FindFormat(&forma)
	if medi == nil {
		panic("media not found")
	}

	// setup RTP/H264->H264 decoder
	rtpDec := forma.CreateDecoder()

	h264RawDec, err := newH264Decoder()
	if err != nil {
		panic(err)
	}
	defer h264RawDec.close()

	// setup H264->MPEGTS muxer
	mpegtsMuxer, err := newMPEGTSMuxer(forma.SafeSPS(), forma.SafePPS())
	if err != nil {
		panic(err)
	}

	// setup a single media
	_, err = c.Setup(medi, baseURL, 0, 0)
	if err != nil {
		panic(err)
	}

	// called when a RTP packet arrives
	c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		// extract NALUs from RTP packets
		// DecodeUntilMarker is necessary for the DTS extractor to work
		nalus, pts, err := rtpDec.DecodeUntilMarker(pkt)
		if err != nil {
			if err != rtph264.ErrNonStartingPacketAndNoPrevious && err != rtph264.ErrMorePacketsNeeded {
				log.Printf("ERR: %v", err)
			}
			return
		}

		saveCount := 0

		for _, nalu := range nalus {
			// convert NALUs into RGBA frames
			img, err := h264RawDec.decode(nalu)
			if err != nil {
				panic(err)
			}

			// wait for a frame
			if img == nil {
				continue
			}

			// convert frame to JPEG and save to file
			err = saveToFile(img)
			if err != nil {
				panic(err)
			}

			saveCount++
			if saveCount == 5 {
				log.Printf("saved 5 images, exiting")
				os.Exit(1)
			}
		}

		// encode H264 NALUs into MPEG-TS
		mpegtsMuxer.encode(nalus, pts)
	})

	// start playing
	_, err = c.Play(nil)
	if err != nil {
		panic(err)
	}

	// wait until a fatal error
	panic(c.Wait())
}

func saveToFile(img image.Image) error {
	// create file
	fname := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10) + ".jpg"
	f, err := os.Create(fname)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	log.Println("saving", fname)

	// convert to jpeg
	return jpeg.Encode(f, img, &jpeg.Options{
		Quality: 60,
	})
}
