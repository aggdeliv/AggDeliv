package mpquic_trans

import (
	"math"
	//	log "github.com/sirupsen/logrus"
	"math/rand"
	"time"

	"mp_quic/ackhandler"
	"mp_quic/internal/protocol"
	"mp_quic/internal/utils"
	"mp_quic/internal/wire"
)

type scheduler struct {
	// XXX Currently round-robin based, inspired from MPTCP scheduler
	quotas      map[protocol.PathID]uint
	tokens      map[protocol.PathID]float64
	curQuotas   map[protocol.PathID]uint
	packetPaths map[protocol.PacketNumber]protocol.PathID
}

func (sch *scheduler) setup() {
	sch.quotas = make(map[protocol.PathID]uint)
	sch.tokens = make(map[protocol.PathID]float64)
	sch.curQuotas = make(map[protocol.PathID]uint)
	sch.packetPaths = make(map[protocol.PacketNumber]protocol.PathID)
}

func (sch *scheduler) getRetransmission(s *session) (hasRetransmission bool, retransmitPacket *ackhandler.Packet, pth *path) {
	// check for retransmissions first
	for {
		// TODO add ability to reinject on another path
		// XXX We need to check on ALL paths if any packet should be first retransmitted
		s.pathsLock.RLock()
	retransmitLoop:
		for _, pthTmp := range s.paths {
			retransmitPacket = pthTmp.sentPacketHandler.DequeuePacketForRetransmission()
			if retransmitPacket != nil {
				pth = pthTmp
				break retransmitLoop
			}
		}
		s.pathsLock.RUnlock()
		if retransmitPacket == nil {
			break
		}
		hasRetransmission = true

		if retransmitPacket.EncryptionLevel != protocol.EncryptionForwardSecure {
			if s.handshakeComplete {
				// Don't retransmit handshake packets when the handshake is complete
				continue
			}
			utils.Debugf("\tDequeueing handshake retransmission for packet 0x%x", retransmitPacket.PacketNumber)
			return
		}
		utils.Debugf("\tDequeueing retransmission of packet 0x%x from path %d", retransmitPacket.PacketNumber, pth.pathID)
		// resend the frames that were in the packet
		for _, frame := range retransmitPacket.GetFramesForRetransmission() {
			switch f := frame.(type) {
			case *wire.StreamFrame:
				s.streamFramer.AddFrameForRetransmission(f)
			case *wire.WindowUpdateFrame:
				// only retransmit WindowUpdates if the stream is not yet closed and the we haven't sent another WindowUpdate with a higher ByteOffset for the stream
				// XXX Should it be adapted to multiple paths?
				currentOffset, err := s.flowControlManager.GetReceiveWindow(f.StreamID)
				if err == nil && f.ByteOffset >= currentOffset {
					s.packer.QueueControlFrame(f, pth)
				}
			case *wire.PathsFrame:
				// Schedule a new PATHS frame to send
				s.schedulePathsFrame()
			default:
				s.packer.QueueControlFrame(frame, pth)
			}
		}
	}
	return
}

func (sch *scheduler) selectPathNew(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var selectedPath *path
	//var lowerRTT time.Duration
	var currentRTT time.Duration
	//selectedPathID := protocol.PathID(255)
	bandwidth := s.BandwidthEstimate()
	//s.probLock.RLock()
	//probability := s.probability
	//s.probLock.RUnlock()
	var fPathID, sPathID protocol.PathID
	var fRTT, sRTT time.Duration
	sRTT = 0
	fRTT = time.Duration(^uint(0) / 2)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()
		//println("curRTT:", currentRTT)

		if currentRTT <= fRTT {
			fRTT = currentRTT
			fPathID = pathID
		}

		if currentRTT >= sRTT {
			sRTT = currentRTT
			sPathID = pathID
		}

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		//if lowerRTT != 0 && currentRTT == 0 {
		//	continue pathLoop
		//}
		//
		//// Case if we have multiple paths unprobed
		//if currentRTT == 0 {
		//	currentQuota, ok := sch.quotas[pathID]
		//	if !ok {
		//		sch.quotas[pathID] = 0
		//		currentQuota = 0
		//	}
		//	lowerQuota, _ := sch.quotas[selectedPathID]
		//	if selectedPath != nil && currentQuota > lowerQuota {
		//		continue pathLoop
		//	}
		//}
		//
		//if currentRTT != 0 && lowerRTT != 0 && selectedPath != nil && currentRTT >= lowerRTT {
		//	continue pathLoop
		//}
		//
		//// Update
		//lowerRTT = currentRTT
		//selectedPath = pth
		//selectedPathID = pathID
	}

	if bandwidth[sPathID]*2*uint64(fRTT) < bandwidth[fPathID]*uint64(sRTT-fRTT) {
		selectedPath = s.paths[fPathID]
	} else {
		selectedPath = s.paths[sPathID]
	}
	println("fRTT:", fRTT, " sRTT:", sRTT)
	println("fPathID:", fPathID, " sPathID:", sPathID)
	println("fBW:", bandwidth[fPathID], " sBW:", bandwidth[sPathID])
	println("selectedPathID:", selectedPath.pathID)
	return selectedPath
}

func (sch *scheduler) selectPathProper(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	if sch.curQuotas == nil {
		sch.setup()
	}

	var selectedPath *path
	//var selectedPathID protocol.PathID

	// return nil if there is only 1 path and the path doesn't work
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
	}

	// take the path with the InitialPathID as the default one
	selectedPath = s.paths[protocol.InitialPathID]

	var totalQuota uint
	lowerDiver := 1.0
	s.probLock.RLock()
	probability := s.probability
	s.probLock.RUnlock()
	//println("prob:", probability[protocol.PathID(1)], probability[protocol.PathID(3)])

	for pathID, _ := range s.paths {
		if pathID != protocol.InitialPathID {
			s.curQuotaLock.RLock()
			totalQuota += sch.curQuotas[pathID]
			s.curQuotaLock.RUnlock()
		}
	}
	totalQuota += 1

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		//if !hasRetransmission && !pth.SendingAllowed() {
		//	continue pathLoop
		//}

		// If this path is potentially failed, don't consider it for sending
		//if pth.potentiallyFailed.Get() {
		//	continue pathLoop
		//}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		divergence1 := 0.0
		divergence2 := 0.0
		for pID, _ := range s.paths {
			if pID != protocol.InitialPathID {
				s.curQuotaLock.Lock()
				quota, ok := sch.curQuotas[pID]
				if !ok {
					sch.curQuotas[pID] = 0
					quota = 0
				}
				s.curQuotaLock.Unlock()
				if pID == pathID {
					quota += 1
				}
				//println("quota:", sch.curQuotas[pID], "pathID:", pID)
				if probability[pID] != 0 {
					//println("...prob:", probability[pID])
					//println("...quota:", quota)
					//println("...totalQuota:", totalQuota)
					divergence1 += probability[pID] * (math.Log2(2*probability[pID]) - math.Log2(probability[pID]+float64(quota)/float64(totalQuota)))
					//tmp := probability[pID] * (math.Log2(2*probability[pID]) - math.Log2(probability[pID]+float64(quota)/float64(totalQuota)))
					//println("...tmp:", tmp)
					divergence2 += float64(quota) / float64(totalQuota) * (math.Log2(2*float64(quota)/float64(totalQuota)) - math.Log2(probability[pID]+float64(quota)/float64(totalQuota)))
				}
			}
			//println("diver1:=", divergence1, "diver2:", divergence2)
		}

		//rand.Seed(time.Now().UnixNano())
		//divergence := 0.01 * float64(rand.Int63n(100))
		divergence := 0.5*divergence1 + 0.5*divergence2
		//println("divergence:", divergence, "pathID:", pathID)
		if divergence < lowerDiver {
			selectedPath = pth
			lowerDiver = divergence
		}
	}
	//println("selectedPathID:", selectedPath.pathID)
	return selectedPath
}

func (sch *scheduler) selectPathToken(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	if sch.quotas == nil {
		sch.setup()
	}

	var selectedPath *path
	coefficient := 10.0
	//var selectedPathID protocol.PathID

	// return nil if there is only 1 path and the path doesn't work
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
	}

	// take the path with the InitialPathID as the default one
	selectedPath = s.paths[protocol.InitialPathID]

	s.probLock.RLock()
	probability := s.probability
	/*****/
	//	probability[protocol.PathID(1)] = 0.3
	//	probability[protocol.PathID(3)] = 0.7
	/*****/
	s.probLock.RUnlock()

	maxToken := 0.0
	for pathID, _ := range s.paths {
		if pathID != protocol.InitialPathID {
			val, ok := sch.tokens[pathID]
			if ok && val > maxToken {
				maxToken = val
			}
		}
	}
	//println("maxToken:", maxToken)
	//println("len:", len(s.paths))
	if maxToken < 1.0 {
		for pathID, _ := range s.paths {
			if pathID != protocol.InitialPathID {
				_, ok := sch.tokens[pathID]
				if false == ok {
					sch.tokens[pathID] = 0
				}
				sch.tokens[pathID] += probability[pathID] * coefficient
				//println("token:", sch.tokens[pathID], "prob:", probability[pathID])
			}
		}
	}

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, don't consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		if sch.tokens[pathID] >= 1.0 {
			selectedPath = pth
			sch.tokens[pathID] -= 1.0
			break
		}
	}
	println("selected path ID:", selectedPath.pathID)
	return selectedPath
}

func (sch *scheduler) selectPathLargeBandwidth(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	//println("selectPathLargeBandwidth(...)")

	var selectedPath *path
	//var selectedPathID protocol.PathID

	// return nil if there is only 1 path and the path doesn't work
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
	}

	// take the path with the InitialPathID as the default one
	selectedPath = s.paths[protocol.InitialPathID]

	thresh := rand.Float64()
	hist := 0.0
	s.probLock.RLock()
	//println("the following is the probability. n:", len(s.probability))
	//probability := make(map[protocol.PathID]float64)
	probability := s.probability
	//log.Infof("num. of probability: %v", len(s.probability))

	/*totalProb := 0.0
	nProb := 1
	for key, val := range s.probability {
		totalProb += val/2.0
		probability[key] = val/2.0
		nProb += 1
	}
	for key, _ := range probability {
		probability[key] += (totalProb/float64(nProb))
	}*/
	s.probLock.RUnlock()
	//for key, val := range probability {
	//	println("pathID:", key, ", prob:", val)
	//}

	// TODO cope with decreasing number of paths (needed?)

	// Max possible value for lowerQuota at the beginning
	//	lowerQuota = ^uint(0)

pathLoop:
	for pathID, pth := range s.paths {
		if pathID != protocol.InitialPathID {
			hist += probability[pathID]
		}
		//println("thresh:", thresh, ", hist:", hist)

		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, don't consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		if hist > thresh {
			selectedPath = pth
			//		selectedPathID = pathID
			break pathLoop
		}
	}
	//println("selectedPathID:", selectedPath.pathID)
	return selectedPath

}

func (sch *scheduler) selectPathRoundRobin(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	s.BandwidthEstimate()
	if sch.quotas == nil {
		sch.setup()
	}

	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// TODO cope with decreasing number of paths (needed?)
	var selectedPath *path
	var lowerQuota, currentQuota uint
	var ok bool
	//var selectedPathID protocol.PathID

	// Max possible value for lowerQuota at the beginning
	lowerQuota = ^uint(0)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, don't consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentQuota, ok = sch.quotas[pathID]
		if !ok {
			sch.quotas[pathID] = 0
			currentQuota = 0
		}

		if currentQuota < lowerQuota {
			selectedPath = pth
			//		selectedPathID = pathID
			lowerQuota = currentQuota
		}
	}
	//println("selectedPathID:", selectedPathID)
	return selectedPath

}

func (sch *scheduler) selectPathLowLatency(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var selectedPath *path
	var lowerRTT time.Duration
	var currentRTT time.Duration
	selectedPathID := protocol.PathID(255)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		if lowerRTT != 0 && currentRTT == 0 {
			continue pathLoop
		}

		// Case if we have multiple paths unprobed
		if currentRTT == 0 {
			currentQuota, ok := sch.quotas[pathID]
			if !ok {
				sch.quotas[pathID] = 0
				currentQuota = 0
			}
			lowerQuota, _ := sch.quotas[selectedPathID]
			if selectedPath != nil && currentQuota > lowerQuota {
				continue pathLoop
			}
		}

		if currentRTT != 0 && lowerRTT != 0 && selectedPath != nil && currentRTT >= lowerRTT {
			continue pathLoop
		}

		// Update
		lowerRTT = currentRTT
		selectedPath = pth
		selectedPathID = pathID
	}

	return selectedPath
}

// Lock of s.paths must be held
func (sch *scheduler) selectPath(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Currently round-robin
	// TODO select the right scheduler dynamically
	// revision start
	//return sch.selectPathNew(s, hasRetransmission, hasStreamRetransmission, fromPth)
	//return sch.selectPathProper(s, hasRetransmission, hasStreamRetransmission, fromPth)
	//return sch.selectPathToken(s, hasRetransmission, hasStreamRetransmission, fromPth)
	return sch.selectPathLargeBandwidth(s, hasRetransmission, hasStreamRetransmission, fromPth)
	// revision end
	//return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	//return sch.selectPathRoundRobin(s, hasRetransmission, hasStreamRetransmission, fromPth)
}

// Lock of s.paths must be free (in case of log print)
func (sch *scheduler) performPacketSending(s *session, windowUpdateFrames []*wire.WindowUpdateFrame, pth *path) (*ackhandler.Packet, bool, error) {
	// add a retransmittable frame
	if pth.sentPacketHandler.ShouldSendRetransmittablePacket() {
		s.packer.QueueControlFrame(&wire.PingFrame{}, pth)
	}
	packet, err := s.packer.PackPacket(pth)
	if err != nil || packet == nil {
		return nil, false, err
	}
	if err = s.sendPackedPacket(packet, pth); err != nil {
		return nil, false, err
	}

	// send every window update twice
	for _, f := range windowUpdateFrames {
		s.packer.QueueControlFrame(f, pth)
	}

	// Packet sent, so update its quota
	sch.quotas[pth.pathID]++
	sch.packetPaths[packet.number] = pth.pathID
	s.curQuotaLock.Lock()
	sch.curQuotas[pth.pathID]++
	s.curQuotaLock.Unlock()
	//println("Quota++:", sch.curQuotas[pth.pathID], "pathID:", pth.pathID)

	// Provide some logging if it is the last packet
	for _, frame := range packet.frames {
		switch frame := frame.(type) {
		case *wire.StreamFrame:
			if frame.FinBit {
				// Last packet to send on the stream, print stats
				s.pathsLock.RLock()
				utils.Infof("Info for stream %x of %x", frame.StreamID, s.connectionID)
				for pathID, pth := range s.paths {
					sntPkts, sntRetrans, sntLost := pth.sentPacketHandler.GetStatistics()
					rcvPkts := pth.receivedPacketHandler.GetStatistics()
					utils.Infof("Path %x: sent %d retrans %d lost %d; rcv %d rtt %v", pathID, sntPkts, sntRetrans, sntLost, rcvPkts, pth.rttStats.SmoothedRTT())
				}
				s.pathsLock.RUnlock()
			}
		default:
		}
	}

	pkt := &ackhandler.Packet{
		PacketNumber:    packet.number,
		Frames:          packet.frames,
		Length:          protocol.ByteCount(len(packet.raw)),
		EncryptionLevel: packet.encryptionLevel,
	}

	return pkt, true, nil
}

// Lock of s.paths must be free
func (sch *scheduler) ackRemainingPaths(s *session, totalWindowUpdateFrames []*wire.WindowUpdateFrame) error {
	// Either we run out of data, or CWIN of usable paths are full
	// Send ACKs on paths not yet used, if needed. Either we have no data to send and
	// it will be a pure ACK, or we will have data in it, but the CWIN should then
	// not be an issue.
	s.pathsLock.RLock()
	defer s.pathsLock.RUnlock()
	// get WindowUpdate frames
	// this call triggers the flow controller to increase the flow control windows, if necessary
	windowUpdateFrames := totalWindowUpdateFrames
	if len(windowUpdateFrames) == 0 {
		windowUpdateFrames = s.getWindowUpdateFrames(s.peerBlocked)
	}
	for _, pthTmp := range s.paths {
		ackTmp := pthTmp.GetAckFrame()
		for _, wuf := range windowUpdateFrames {
			s.packer.QueueControlFrame(wuf, pthTmp)
		}
		if ackTmp != nil || len(windowUpdateFrames) > 0 {
			if pthTmp.pathID == protocol.InitialPathID && ackTmp == nil {
				continue
			}
			swf := pthTmp.GetStopWaitingFrame(false)
			if swf != nil {
				s.packer.QueueControlFrame(swf, pthTmp)
			}
			s.packer.QueueControlFrame(ackTmp, pthTmp)
			// XXX (QDC) should we instead call PackPacket to provides WUFs?
			var packet *packedPacket
			var err error
			if ackTmp != nil {
				// Avoid internal error bug
				packet, err = s.packer.PackAckPacket(pthTmp)
			} else {
				packet, err = s.packer.PackPacket(pthTmp)
			}
			if err != nil {
				return err
			}
			err = s.sendPackedPacket(packet, pthTmp)
			if err != nil {
				return err
			}
		}
	}
	s.peerBlocked = false
	return nil
}

func (sch *scheduler) sendPacket(s *session) error {
	var pth *path

	// Update leastUnacked value of paths
	s.pathsLock.RLock()
	for _, pthTmp := range s.paths {
		pthTmp.SetLeastUnacked(pthTmp.sentPacketHandler.GetLeastUnacked())
	}
	s.pathsLock.RUnlock()

	// get WindowUpdate frames
	// this call triggers the flow controller to increase the flow control windows, if necessary
	windowUpdateFrames := s.getWindowUpdateFrames(false)
	for _, wuf := range windowUpdateFrames {
		s.packer.QueueControlFrame(wuf, pth)
	}

	// Repeatedly try sending until we don't have any more data, or run out of the congestion window
	for {
		// We first check for retransmissions
		hasRetransmission, retransmitHandshakePacket, fromPth := sch.getRetransmission(s)
		// XXX There might still be some stream frames to be retransmitted
		hasStreamRetransmission := s.streamFramer.HasFramesForRetransmission()

		// Select the path here
		s.pathsLock.RLock()
		pth = sch.selectPath(s, hasRetransmission, hasStreamRetransmission, fromPth)
		s.pathsLock.RUnlock()

		// XXX No more path available, should we have a new QUIC error message?
		if pth == nil {
			windowUpdateFrames := s.getWindowUpdateFrames(false)
			return sch.ackRemainingPaths(s, windowUpdateFrames)
		}

		// If we have an handshake packet retransmission, do it directly
		if hasRetransmission && retransmitHandshakePacket != nil {
			s.packer.QueueControlFrame(pth.sentPacketHandler.GetStopWaitingFrame(true), pth)
			packet, err := s.packer.PackHandshakeRetransmission(retransmitHandshakePacket, pth)
			if err != nil {
				return err
			}
			if err = s.sendPackedPacket(packet, pth); err != nil {
				return err
			}
			continue
		}

		// XXX Some automatic ACK generation should be done someway
		var ack *wire.AckFrame

		ack = pth.GetAckFrame()
		if ack != nil {
			s.packer.QueueControlFrame(ack, pth)
		}
		if ack != nil || hasStreamRetransmission {
			swf := pth.sentPacketHandler.GetStopWaitingFrame(hasStreamRetransmission)
			if swf != nil {
				s.packer.QueueControlFrame(swf, pth)
			}
		}

		// Also add CLOSE_PATH frames, if any
		for cpf := s.streamFramer.PopClosePathFrame(); cpf != nil; cpf = s.streamFramer.PopClosePathFrame() {
			s.packer.QueueControlFrame(cpf, pth)
		}

		// Also add ADD ADDRESS frames, if any
		for aaf := s.streamFramer.PopAddAddressFrame(); aaf != nil; aaf = s.streamFramer.PopAddAddressFrame() {
			s.packer.QueueControlFrame(aaf, pth)
		}

		// Also add PATHS frames, if any
		for pf := s.streamFramer.PopPathsFrame(); pf != nil; pf = s.streamFramer.PopPathsFrame() {
			s.packer.QueueControlFrame(pf, pth)
		}

		pkt, sent, err := sch.performPacketSending(s, windowUpdateFrames, pth)
		if err != nil {
			return err
		}
		windowUpdateFrames = nil
		if !sent {
			// Prevent sending empty packets
			return sch.ackRemainingPaths(s, windowUpdateFrames)
		}

		// Duplicate traffic when it was sent on an unknown performing path
		// FIXME adapt for new paths coming during the connection
		if pth.rttStats.SmoothedRTT() == 0 {
			currentQuota := sch.quotas[pth.pathID]
			// Was the packet duplicated on all potential paths?
		duplicateLoop:
			for pathID, tmpPth := range s.paths {
				if pathID == protocol.InitialPathID || pathID == pth.pathID {
					continue
				}
				if sch.quotas[pathID] < currentQuota && tmpPth.sentPacketHandler.SendingAllowed() {
					// Duplicate it
					pth.sentPacketHandler.DuplicatePacket(pkt)
					break duplicateLoop
				}
			}
		}

		// And try pinging on potentially failed paths
		if fromPth != nil && fromPth.potentiallyFailed.Get() {
			err = s.sendPing(fromPth)
			if err != nil {
				return err
			}
		}
	}
}
