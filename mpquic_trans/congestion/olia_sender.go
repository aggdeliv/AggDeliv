package congestion

import (
	"time"

	"mp_quic/internal/protocol"
	"mp_quic/internal/utils"
)

type OliaSender struct {
	hybridSlowStart HybridSlowStart
	prr             PrrSender
	rttStats        *RTTStats
	stats           connectionStats
	olia            *Olia
	oliaSenders     map[protocol.PathID]*OliaSender

	// Track the largest packet that has been sent.
	largestSentPacketNumber protocol.PacketNumber

	// Track the largest packet that has been acked.
	largestAckedPacketNumber protocol.PacketNumber

	// Track the largest packet number outstanding when a CWND cutbacks occurs.
	largestSentAtLastCutback protocol.PacketNumber

	// Congestion window in packets.
	congestionWindow protocol.PacketNumber

	// Slow start congestion window in packets, aka ssthresh.
	slowstartThreshold protocol.PacketNumber

	// Whether the last loss event caused us to exit slowstart.
	// Used for stats collection of slowstartPacketsLost
	lastCutbackExitedSlowstart bool

	// When true, texist slow start with large cutback of congestion window.
	slowStartLargeReduction bool

	// Minimum congestion window in packets.
	minCongestionWindow protocol.PacketNumber

	// Maximum number of outstanding packets for tcp.
	maxTCPCongestionWindow protocol.PacketNumber

	// Number of connections to simulate
	numConnections int

	// ACK counter for the Reno implementation
	congestionWindowCount protocol.ByteCount

	initialCongestionWindow    protocol.PacketNumber
	initialMaxCongestionWindow protocol.PacketNumber

	/***** added by zhao zongyi *****/
	TmpBwt              Bandwidth
	tmpCongestionWindow protocol.PacketNumber
	/**********/
}

func NewOliaSender(oliaSenders map[protocol.PathID]*OliaSender, rttStats *RTTStats, initialCongestionWindow, initialMaxCongestionWindow protocol.PacketNumber) SendAlgorithmWithDebugInfo {
	//println("NewOliaSender(...)")
	return &OliaSender{
		rttStats:                   rttStats,
		initialCongestionWindow:    initialCongestionWindow,
		initialMaxCongestionWindow: initialMaxCongestionWindow,
		congestionWindow:           initialCongestionWindow,
		minCongestionWindow:        defaultMinimumCongestionWindow,
		slowstartThreshold:         initialMaxCongestionWindow,
		maxTCPCongestionWindow:     initialMaxCongestionWindow,
		numConnections:             defaultNumConnections,
		olia:                       NewOlia(0),
		oliaSenders:                oliaSenders,
		/***** added by zhao zongyi *****/
		tmpCongestionWindow: initialCongestionWindow,
		/**********/
	}
}

func (o *OliaSender) TimeUntilSend(now time.Time, bytesInFlight protocol.ByteCount) time.Duration {
	//println("TimeUntilSend(...)")
	if o.InRecovery() {
		// PRR is used when in recovery.
		return o.prr.TimeUntilSend(o.GetCongestionWindow(), bytesInFlight, o.GetSlowStartThreshold())
	}
	if o.GetCongestionWindow() > bytesInFlight {
		return 0
	}
	return utils.InfDuration
}

func (o *OliaSender) OnPacketSent(sentTime time.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) bool {
	//println("OnPacketSent(...)")
	// Only update bytesInFlight for data packets.
	if !isRetransmittable {
		return false
	}
	if o.InRecovery() {
		// PRR is used when in recovery.
		o.prr.OnPacketSent(bytes)
	}
	o.largestSentPacketNumber = packetNumber
	o.hybridSlowStart.OnPacketSent(packetNumber)
	return true
}

func (o *OliaSender) GetCongestionWindow() protocol.ByteCount {
	//println("GetCongestWindow()")
	res := protocol.ByteCount(o.congestionWindow) * protocol.DefaultTCPMSS
	//println("congestionWindow:", res)
	//return protocol.ByteCount(o.congestionWindow) * protocol.DefaultTCPMSS
	return res
}

func (o *OliaSender) PrintCongestionWindow() {
	//println("congestionWindow:", o.congestionWindow, ", tmpCongestionWindow:", o.tmpCongestionWindow)
}

func (o *OliaSender) GetSlowStartThreshold() protocol.ByteCount {
	//println("GetSlowStartThreshold()")
	return protocol.ByteCount(o.slowstartThreshold) * protocol.DefaultTCPMSS
}

func (o *OliaSender) ExitSlowstart() {
	//println("ExitSlowstart()")
	o.slowstartThreshold = o.congestionWindow
}

func (o *OliaSender) MaybeExitSlowStart() {
	//println("MaybeExitSlowStart()")
	if o.InSlowStart() && o.hybridSlowStart.ShouldExitSlowStart(o.rttStats.LatestRTT(), o.rttStats.MinRTT(), o.GetCongestionWindow()/protocol.DefaultTCPMSS) {
		o.ExitSlowstart()
	}
}

func (o *OliaSender) isCwndLimited(bytesInFlight protocol.ByteCount) bool {
	//println("isCwndLimited(...)")
	congestionWindow := o.GetCongestionWindow()
	if bytesInFlight >= congestionWindow {
		return true
	}
	availableBytes := congestionWindow - bytesInFlight
	slowStartLimited := o.InSlowStart() && bytesInFlight > congestionWindow/2
	return slowStartLimited || availableBytes <= maxBurstBytes
}

func getMaxCwnd(m map[protocol.PathID]*OliaSender) protocol.PacketNumber {
	//println("getMaxCwnd(...)")
	var bestCwnd protocol.PacketNumber
	for _, os := range m {
		// TODO should we care about fast retransmit and RFC5681?
		bestCwnd = utils.MaxPacketNumber(bestCwnd, os.congestionWindow)
	}
	return bestCwnd
}

func getRate(m map[protocol.PathID]*OliaSender, pathRTT time.Duration) protocol.ByteCount {
	//println("getRate(...)")
	// We have to avoid a zero rate because it is used as a divisor
	var rate protocol.ByteCount = 1
	var tmpCwnd protocol.PacketNumber
	var scaledNum uint64
	for _, os := range m {
		tmpCwnd = os.congestionWindow
		scaledNum = oliaScale(uint64(tmpCwnd), scale) * uint64(pathRTT.Nanoseconds())
		if os.rttStats.SmoothedRTT() != time.Duration(0) {
			// XXX In MPTCP, we have an estimate of the RTT because of the handshake, not in QUIC...
			rate += protocol.ByteCount(scaledNum / uint64(os.rttStats.SmoothedRTT().Nanoseconds()))
		}
	}
	rate *= rate
	return rate
}

func (o *OliaSender) getEpsilon() {
	//println("getEpsilon()")
	// TODOi
	var tmpRTT time.Duration
	var tmpBytes protocol.ByteCount

	var tmpCwnd protocol.PacketNumber

	var bestRTT time.Duration
	var bestBytes protocol.ByteCount

	var M uint8
	var BNotM uint8

	// TODO: integrate this in the following loop - we just want to iterate once
	maxCwnd := getMaxCwnd(o.oliaSenders)
	for _, os := range o.oliaSenders {
		tmpRTT = os.rttStats.SmoothedRTT() * os.rttStats.SmoothedRTT()
		tmpBytes = os.olia.SmoothedBytesBetweenLosses()
		if int64(tmpBytes)*bestRTT.Nanoseconds() >= int64(bestBytes)*tmpRTT.Nanoseconds() {
			bestRTT = tmpRTT
			bestBytes = tmpBytes
		}
	}

	// TODO: integrate this here in getMaxCwnd and in the previous loop
	// Find the size of M and BNotM
	for _, os := range o.oliaSenders {
		tmpCwnd = os.congestionWindow
		if tmpCwnd == maxCwnd {
			M++
		} else {
			tmpRTT = os.rttStats.SmoothedRTT() * os.rttStats.SmoothedRTT()
			tmpBytes = os.olia.SmoothedBytesBetweenLosses()
			if int64(tmpBytes)*bestRTT.Nanoseconds() >= int64(bestBytes)*tmpRTT.Nanoseconds() {
				BNotM++
			}
		}
	}

	// Check if the path is in M or BNotM and set the value of epsilon accordingly
	for _, os := range o.oliaSenders {
		if BNotM == 0 {
			os.olia.epsilonNum = 0
			os.olia.epsilonDen = 1
		} else {
			tmpRTT = os.rttStats.SmoothedRTT() * os.rttStats.SmoothedRTT()
			tmpBytes = os.olia.SmoothedBytesBetweenLosses()
			tmpCwnd = os.congestionWindow

			if tmpCwnd < maxCwnd && int64(tmpBytes)*bestRTT.Nanoseconds() >= int64(bestBytes)*tmpRTT.Nanoseconds() {
				os.olia.epsilonNum = 1
				os.olia.epsilonDen = uint32(len(o.oliaSenders)) * uint32(BNotM)
			} else if tmpCwnd == maxCwnd {
				os.olia.epsilonNum = -1
				os.olia.epsilonDen = uint32(len(o.oliaSenders)) * uint32(M)
			} else {
				os.olia.epsilonNum = 0
				os.olia.epsilonDen = 1
			}
		}
	}
}

func (o *OliaSender) maybeIncreaseCwnd(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	//println("cwnd:", o.congestionWindow)
	//println("maybeIncreaseCwnd(...)")
	// Do not increase the congestion window unless the sender is close to using
	// the current window.
	if !o.isCwndLimited(bytesInFlight) {
		//	println("p1")
		return
	}
	if o.congestionWindow >= o.maxTCPCongestionWindow {
		//	println("p2")
		return
	}
	if o.InSlowStart() {
		// TCP slow start, exponential growth, increase by one for each ACK.
		//	println("p3")
		o.congestionWindow++
		//o.congestionWindow += 2
		//	return
	} else {
		//	println("p4")
		o.getEpsilon()
		rate := getRate(o.oliaSenders, o.rttStats.SmoothedRTT())
		cwndScaled := oliaScale(uint64(o.congestionWindow), scale)
		o.congestionWindow = utils.MinPacketNumber(o.maxTCPCongestionWindow, o.olia.CongestionWindowAfterAck(o.congestionWindow, rate, cwndScaled))
	}
	/***** added by zhao zongyi *****/
	//o.TmpBwt = o.BandwidthEstimate()
	o.UpdateBandwidth()
	o.tmpCongestionWindow = o.congestionWindow
	//println("#1:", o.TmpBwt, o.congestionWindow)
	/**********/
}

func (o *OliaSender) UpdateBandwidth() {
	o.TmpBwt = o.BandwidthEstimate()
}

func (o *OliaSender) OnPacketAcked(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	//println("OnPacketAcked(...)")
	o.largestAckedPacketNumber = utils.MaxPacketNumber(ackedPacketNumber, o.largestAckedPacketNumber)
	if o.InRecovery() {
		// PRR is used when in recovery
		o.prr.OnPacketAcked(ackedBytes)
		return
	}
	o.olia.UpdateAckedSinceLastLoss(ackedBytes)
	o.maybeIncreaseCwnd(ackedPacketNumber, ackedBytes, bytesInFlight)
	if o.InSlowStart() {
		o.hybridSlowStart.OnPacketAcked(ackedPacketNumber)
	}
}

func (o *OliaSender) OnPacketLost(packetNumber protocol.PacketNumber, lostBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	//println("OnPacketLost(...)")
	// TCP NewReno (RFC6582) says that once a loss occurs, any losses in packets
	// already sent should be treated as a single loss event, since it's expected.
	if packetNumber <= o.largestSentAtLastCutback {
		if o.lastCutbackExitedSlowstart {
			o.stats.slowstartPacketsLost++
			o.stats.slowstartBytesLost += lostBytes
			if o.slowStartLargeReduction {
				if o.stats.slowstartPacketsLost == 1 || (o.stats.slowstartBytesLost/protocol.DefaultTCPMSS) > (o.stats.slowstartBytesLost-lostBytes)/protocol.DefaultTCPMSS {
					// Reduce congestion window by 1 for every mss of bytes lost.
					o.congestionWindow = utils.MaxPacketNumber(o.congestionWindow-1, o.minCongestionWindow)
					/***** added by zhao zongyi *****/
					//o.TmpBwt = o.BandwidthEstimate()
					o.UpdateBandwidth()
					o.tmpCongestionWindow = o.congestionWindow
					println("#3")
					/**********/
				}
				o.slowstartThreshold = o.congestionWindow
			}
		}
		return
	}
	o.lastCutbackExitedSlowstart = o.InSlowStart()
	if o.InSlowStart() {
		o.stats.slowstartPacketsLost++
	}

	o.prr.OnPacketLost(bytesInFlight)
	o.olia.OnPacketLost()

	// TODO(chromium): Separate out all of slow start into a separate class.
	if o.slowStartLargeReduction && o.InSlowStart() {
		o.congestionWindow = o.congestionWindow - 1
	} else {
		o.congestionWindow = protocol.PacketNumber(float32(o.congestionWindow) * o.RenoBeta())
	}
	// Enforce a minimum congestion window.
	if o.congestionWindow < o.minCongestionWindow {
		o.congestionWindow = o.minCongestionWindow
	}
	/***** added by zhao zongyi **********/
	//	o.TmpBwt = o.BandwidthEstimate()
	o.UpdateBandwidth()
	o.tmpCongestionWindow = o.congestionWindow
	println("#4")
	/**********/
	o.slowstartThreshold = o.congestionWindow
	o.largestSentAtLastCutback = o.largestSentPacketNumber
	// reset packet count from congestion avoidance mode. We start
	// counting again when we're out of recovery.
	o.congestionWindowCount = 0
}

func (o *OliaSender) SetNumEmulatedConnections(n int) {
	//println("SetNumEmulateConnections(...)")
	o.numConnections = utils.Max(n, 1)
	// TODO should it be done also for OLIA?
}

// OnRetransmissionTimeout is called on an retransmission timeout
func (o *OliaSender) OnRetransmissionTimeout(packetsRetransmitted bool) {
	//println("OnRetransmissionTimeout(...)")
	o.largestSentAtLastCutback = 0
	if !packetsRetransmitted {
		return
	}
	o.hybridSlowStart.Restart()
	o.olia.Reset()
	o.slowstartThreshold = o.congestionWindow / 2
	o.congestionWindow = o.minCongestionWindow
	/***** added by zhao zongyi **********/
	//o.TmpBwt = o.BandwidthEstimate()
	o.UpdateBandwidth()
	o.tmpCongestionWindow = o.congestionWindow
	println("#5")
	/**********/
}

func (o *OliaSender) OnConnectionMigration() {
	//println("OnConnectionMigration()")
	o.hybridSlowStart.Restart()
	o.prr = PrrSender{}
	o.largestSentPacketNumber = 0
	o.largestAckedPacketNumber = 0
	o.largestSentAtLastCutback = 0
	o.lastCutbackExitedSlowstart = false
	o.olia.Reset()
	o.congestionWindowCount = 0
	o.congestionWindow = o.initialCongestionWindow
	o.slowstartThreshold = o.initialMaxCongestionWindow
	o.maxTCPCongestionWindow = o.initialMaxCongestionWindow
	/***** added by zhao zongyi **********/
	//o.TmpBwt = o.BandwidthEstimate()
	o.UpdateBandwidth()
	o.tmpCongestionWindow = o.congestionWindow
	println("#6")
	/**********/
}

// RetransmissionDelay gives the RTO retransmission time
func (o *OliaSender) RetransmissionDelay() time.Duration {
	//println("RetransmissionDelay()")
	if o.rttStats.SmoothedRTT() == 0 {
		return 0
	}
	return o.rttStats.SmoothedRTT() + o.rttStats.MeanDeviation()*4
}

func (o *OliaSender) SmoothedRTT() time.Duration {
	//println("SmoothedRTT()")
	return o.rttStats.SmoothedRTT()
}

func (o *OliaSender) SetSlowStartLargeReduction(enabled bool) {
	//println("SetSlowStartLargeReduction(...)")
	o.slowStartLargeReduction = enabled
}

func (o *OliaSender) BandwidthEstimate() Bandwidth {
	srtt := o.rttStats.SmoothedRTT()
	if srtt == 0 {
		// If we haven't measured an rtt, the bandwidth estimate is unknown.
		//println("srtt: 0, cwnd:", o.GetCongestionWindow())
		return 0
	}
	res := BandwidthFromDelta(o.GetCongestionWindow(), srtt)
	//println("cwnd:", o.GetCongestionWindow(), ", srtt:", srtt, ", bandwidth:", res)
	return res
}

// HybridSlowStart returns the hybrid slow start instance for testing
func (o *OliaSender) HybridSlowStart() *HybridSlowStart {
	//println("HybridSlowStart()")
	return &o.hybridSlowStart
}

func (o *OliaSender) SlowstartThreshold() protocol.PacketNumber {
	//println("SlowstartThreshold()")
	return o.slowstartThreshold
}

func (o *OliaSender) RenoBeta() float32 {
	//println("RenoBeta()")
	// kNConnectionBeta is the backoff factor after loss for our N-connection
	// emulation, which emulates the effective backoff of an ensemble of N
	// TCP-Reno connections on a single loss event. The effective multiplier is
	// computed as:
	return (float32(o.numConnections) - 1. + renoBeta) / float32(o.numConnections)
}

func (o *OliaSender) InRecovery() bool {
	//println("InRecovery()")
	return o.largestAckedPacketNumber <= o.largestSentAtLastCutback && o.largestAckedPacketNumber != 0
}

func (o *OliaSender) InSlowStart() bool {
	//println("InSlowStart()")
	return o.GetCongestionWindow() < o.GetSlowStartThreshold()
}
