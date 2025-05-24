package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// Helper to read base64 SDP from stdin
func readSDP() string {
	fmt.Println("Paste base64 encoded SDP offer below and press enter:")
	reader := bufio.NewReader(os.Stdin)
	encodedOffer, _ := reader.ReadString('\n')
	encodedOffer = strings.TrimSpace(encodedOffer)

	decodedOffer, err := base64.StdEncoding.DecodeString(encodedOffer)
	if err != nil {
		panic(err)
	}
	return string(decodedOffer)
}

// Helper to encode SDP to base64 and print
func printSDP(sdp string) {
	encodedSDP := base64.StdEncoding.EncodeToString([]byte(sdp))
	fmt.Println("\nCopy base64 encoded SDP answer below:")
	fmt.Println(encodedSDP)
}

func main() {
	// 1. MediaEngine creation (using defaults via NewPeerConnection)
	// By default, NewPeerConnection comes with a default set of codecs and interceptors [4].
	// If you wanted to customize codecs or interceptors, you would create an API object first:
	/*
		mediaEngine := &webrtc.MediaEngine{}
		// Register default codecs (includes Opus, PCM, H264, VP8, VP9 etc.) [1, 2, 8]
		if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
			panic(err)
		}
		// You could also register specific codecs one by one:
		// mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{...}, webrtc.RTPCodecTypeAudio) [5]

		interceptorRegistry := &interceptor.Registry{}
		// Register default interceptors (NACK, TWCC, etc.) [6, 7]
		if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
			panic(err)
		}

		api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine), webrtc.WithInterceptorRegistry(interceptorRegistry)) [9]

		// Then create peerConnection using:
		// pc, err := api.NewPeerConnection(webrtc.Configuration{}) [3]
	*/

	// Use the simpler approach for this example: NewPeerConnection uses defaults [4].
	// This configuration will include default codecs like Opus (audio) and VP8/VP9/H264 (video) [1, 2].
	// It also includes default interceptors [4].
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}
	defer peerConnection.Close() // Ensure the connection is closed

	// Optional: Add event handlers for state changes [10]
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed to %s\n", connectionState.String())
	})
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed to %s\n", state.String())
		if state == webrtc.PeerConnectionStateConnected {
			fmt.Println("PeerConnection is connected!")
			// This is where you might start sending media if you added tracks
		} else if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			fmt.Println("PeerConnection failed or closed.")
			// Handle connection failure or closure
		}
	})
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %q with protocol %q\n", d.Label(), d.Protocol())
		d.OnOpen(func() { fmt.Println("Data channel opened!") })
		d.OnMessage(func(msg webrtc.DataChannelMessage) { fmt.Printf("Message from DataChannel: %s\n", string(msg.Data)) })
	})

	// 2. Add Local Tracks (for sending data)
	// We need to add tracks *before* creating the answer to include them in the SDP.
	// TrackLocalStaticSample is suitable for sending media decoded from a file or generated.
	// You'd typically get RTPCodecCapability from your MediaEngine configuration.
	// For simplicity, we'll define basic ones that are likely supported by default codecs.
	opusCodec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, RTCPFeedback: nil},
		// PayloadType:        111, // Common payload type for Opus
	}
	vp8Codec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000, RTCPFeedback: []webrtc.RTCPFeedback{{Type: "transport-cc"}, {Type: "nack", Parameter: "pli"}}}, // Example RTCP feedback [14]
		// PayloadType:        96, // Common payload type for VP8
	}

	// Create local audio track
	audioTrack, err := webrtc.NewTrackLocalStaticSample(opusCodec.RTPCodecCapability, "audio", "pion-stream")
	if err != nil {
		panic(err)
	}
	_, err = peerConnection.AddTrack(audioTrack) // Add track to the peer connection [10]
	if err != nil {
		panic(err)
	}

	// Create local video track
	videoTrack, err := webrtc.NewTrackLocalStaticSample(vp8Codec.RTPCodecCapability, "video", "pion-stream")
	if err != nil {
		panic(err)
	}
	_, err = peerConnection.AddTrack(videoTrack) // Add track to the peer connection [10]
	if err != nil {
		panic(err)
	}

	fmt.Println("Added audio and video tracks for sending.")

	// Read the SDP Offer from the browser
	// offerSDP := readSDP()
	// fmt.Printf("Received SDP Offer:\n%s\n", offerSDP)

	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	decode(readUntilNewline(), &offer)

	// if err = json.Unmarshal(b, obj); err != nil {
	// 	panic(err)
	// }

	// Create a SessionDescription object from the offer string [16]
	// offer := webrtc.SessionDescription{
	// 	Type: webrtc.SDPTypeOffer,// Specifies this is an offer [17]
	// 	SDP:  offerSDP,
	// }

	// Set the RemoteDescription to the received offer [10, 18]
	// This tells our PeerConnection the capabilities and candidates of the remote peer.
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}
	fmt.Println("Set remote description.")

	// 3. Create the Answer
	// Generate a new answer based on the remote offer and our local capabilities (from the MediaEngine) [10, 19]
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("Created answer.")

	// Handle ICE gathering complete (important for sending the *final* answer with all candidates) [6, 7]
	// This promise channel closes when ICE candidate gathering is finished for this PeerConnection [6, 7].
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Set the LocalDescription to the generated answer [10, 18]
	// This makes our PeerConnection aware of its own configuration for the connection.
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}
	fmt.Println("Set local description.")

	// Wait for ICE gathering to complete before printing the SDP [6, 7]
	// The final SDP with all candidates is available in LocalDescription() after this. [10, 20]
	fmt.Println("Waiting for ICE gathering to complete...")
	<-gatherComplete
	fmt.Println("ICE gathering complete.")

	// Get the final LocalDescription (which now includes ICE candidates) [10, 20]
	finalAnswer := peerConnection.LocalDescription()

	fmt.Println("Final SDP Answer copy past to browser:")
	// Print the base64 encoded SDP Answer to send back to the browser
	fmt.Println(encode(finalAnswer))
	fmt.Println("SDP answer printed. Send this back to the browser.")

	// 4. Communicate (Send Data)
	// Once connected (PeerConnectionStateConnected), you can start sending data.
	// This part demonstrates *how* you would send, using placeholder samples.
	// In a real application, you'd get audio/video data from a source (microphone, camera, file).
	// We'll use goroutines to simulate sending data on the tracks.

	// Simple function to send dummy samples
	sendDummySamples := func(track *webrtc.TrackLocalStaticSample, interval time.Duration, sampleSize int) {
		fmt.Printf("Starting to send dummy samples for track %s...\n", track.ID())
		for {
			// Create a dummy sample (e.g., silence for audio, black frame for video)
			dummySample := media.Sample{
				Data:     make([]byte, sampleSize), // Zeroed bytes
				Duration: interval,                 // Estimated duration per sample
			}

			// Write the sample to the track
			err := track.WriteSample(dummySample) // Note: WriteSample is not explicitly in the provided excerpts but is a standard Pion API for TrackLocalStaticSample
			if err != nil {
				// Check if the peer connection is closed
				if peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
					fmt.Printf("Track %s: PeerConnection closed, stopping sender.\n", track.ID())
					return
				}
				// Log other errors
				fmt.Printf("Track %s WriteSample error: %v\n", track.ID(), err)
				// Depending on the error, you might break or continue
			}

			// Sleep for the sample duration or a fixed interval
			time.Sleep(interval)
		}
	}

	// Start sending dummy audio and video (adjust intervals and sizes as needed)
	// These intervals and sizes are *rough* estimates and depend heavily on codec and settings.
	// For Opus 48kHz, 20ms is common, sample size varies.
	// For VP8 30fps, ~33ms interval, size varies wildly.
	audioInterval := 20 * time.Millisecond // 50 packets/sec
	audioSampleSize := 100                 // Placeholder size

	videoInterval := 33 * time.Millisecond // ~30 frames/sec
	videoSampleSize := 1000                // Placeholder size

	go sendDummySamples(audioTrack, audioInterval, audioSampleSize)
	go sendDummySamples(videoTrack, videoInterval, videoSampleSize)

	// Keep the main goroutine alive indefinitely until the connection is closed manually or due to error
	fmt.Println("Press Ctrl+C to stop the program.")
	select {} // Block forever
}

// Decode a base64 and unmarshal JSON into a SessionDescription.
func decode(in string, obj *webrtc.SessionDescription) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}

	if err = json.Unmarshal(b, obj); err != nil {
		panic(err)
	}
}

// Read from stdin until we get a newline.
func readUntilNewline() (in string) {
	var err error

	r := bufio.NewReader(os.Stdin)
	for {
		in, err = r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}

		if in = strings.TrimSpace(in); len(in) > 0 {
			break
		}
	}

	fmt.Println("")

	return
}

// JSON encode + base64 a SessionDescription.
func encode(obj *webrtc.SessionDescription) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}
