import CoreBluetooth
import Foundation
import UIKit

/**
 * Bluetooth proximity, Herald-style (docs/proximity.md). This phone advertises KINJO (the
 * token in the local name while open; iOS moves the UUID to the overflow area in the
 * background) with a GATT server: TOKEN (read) and SIGNAL (write + notify). It scans for
 * KINJO, reads Android's service data straight from the advert, and connects to the rest:
 * reads TOKEN, then keeps iPhones connected (up to maxKept, SIGNAL subscribed) and writes them
 * 0x01 + our token after each RSSI read, every keepEvery: a write wakes the other app, so two
 * backgrounded iPhones keep each other running. iOS timers don't fire in the background, so
 * the loop runs on Bluetooth callbacks. 0x02 frames (Android relays) are reported as relays.
 * Main thread only.
 */
final class Ble: NSObject, CBCentralManagerDelegate, CBPeripheralManagerDelegate, CBPeripheralDelegate {
    static let shared = Ble()

    static let kinjo = CBUUID(string: "6b1a0c30-4f9e-4b8e-9c55-2d3c7e1f0a30")
    static let tokenChar = CBUUID(string: "6b1a0c31-4f9e-4b8e-9c55-2d3c7e1f0a30")
    static let signalChar = CBUUID(string: "6b1a0c32-4f9e-4b8e-9c55-2d3c7e1f0a30")
    // second service UUID: its bit in Apple's overflow bitmap is what Android's scan filter looks for (the server's iosBit)
    static let overflowMarker = CBUUID(string: "00000000-0000-0000-0000-000000000071")
    static let reportEvery: TimeInterval = 10, reportFgEvery: TimeInterval = 5, heardFor: TimeInterval = 120
    static let newAfter: TimeInterval = 8, newDelay: TimeInterval = 0.3 // not heard this long: report it within newDelay
    static let keepEvery: TimeInterval = 8, connectFor: TimeInterval = 8
    static let cacheFor: TimeInterval = 600 // a peer's token, by peripheral id (Android addresses rotate ~15 min)
    static let maxConnecting = 3, maxKept = 8, maxSightings = 32

    private struct Known { var tok: String; var at: Date }

    private var cm: CBCentralManager?
    private var pm: CBPeripheralManager?
    private var signal: CBMutableCharacteristic?
    private var subscribers: [CBCentral] = []
    private var on = false, foreground = true, serviceAdded = false, resetServices = false
    private var token: String?, tokenDue = Date.distantPast
    private var timer: Timer?, reportedAt = Date.distantPast, soonAt: Date? // soonAt: someone new, report from then
    private var direct: [String: Int] = [:] // token -> strongest RSSI since the last report (0: over a connection, no RSSI)
    private var relays: Set<String> = []
    private var heardAt: [String: Date] = [:]
    private var known: [UUID: Known] = [:]
    private var connecting: [UUID: CBPeripheral] = [:] // connect started, token not read yet
    private var kept: [UUID: CBPeripheral] = [:] // iPhones we stay connected to
    private var lastRSSI: [UUID: Int] = [:]
    private var failedAt: [UUID: Date] = [:], fails: [UUID: Int] = [:]
    private var askWaiters: [(Bool) -> Void] = []

    var allowed: Bool { CBManager.authorization == .allowedAlways }
    var asked: Bool { CBManager.authorization != .notDetermined }
    var poweredOn: Bool { cm?.state == .poweredOn }
    var heard: Int { heardAt.count }

    private override init() {
        super.init()
        let nc = NotificationCenter.default
        nc.addObserver(self, selector: #selector(toForeground), name: UIApplication.didBecomeActiveNotification, object: nil)
        nc.addObserver(self, selector: #selector(toBackground), name: UIApplication.didEnterBackgroundNotification, object: nil)
    }

    /** iOS's Bluetooth prompt; done(granted) once the person chose (at once if they already did). */
    func ask(_ done: @escaping (Bool) -> Void) {
        guard !asked else { done(allowed); return }
        askWaiters.append(done)
        makeManagers() // the first manager shows the prompt
    }

    /** No-op until allowed and signed in. Also called on a background relaunch. */
    func start() {
        guard allowed, Presence.session != nil else { return }
        foreground = UIApplication.shared.applicationState != .background
        if on { return }
        on = true
        makeManagers()
        timer?.invalidate()
        timer = Timer.scheduledTimer(withTimeInterval: Self.reportFgEvery, repeats: true) { [weak self] _ in self?.step() }
        step()
    }

    func stop() {
        on = false
        timer?.invalidate(); timer = nil
        cm?.stopScan()
        pm?.stopAdvertising()
        for p in Array(connecting.values) + Array(kept.values) { cm?.cancelPeripheralConnection(p) }
        connecting = [:]; kept = [:]; known = [:]; direct = [:]; relays = []; heardAt = [:]; lastRSSI = [:]
        failedAt = [:]; fails = [:]; subscribers = []; iphones = []
        token = nil
        tokenDue = .distantPast
    }

    /** The server's wake (push or socket "scan"): scan again, read every kept peer, report now. */
    func burst() {
        guard on else { return }
        scan()
        kept.values.forEach { $0.readRSSI() }
        reportedAt = .distantPast
        step()
    }

    private func makeManagers() {
        if cm == nil {
            cm = CBCentralManager(delegate: self, queue: nil, options: [
                CBCentralManagerOptionRestoreIdentifierKey: "kinjo.ble.central",
                CBCentralManagerOptionShowPowerAlertKey: false,
            ])
        }
        if pm == nil {
            pm = CBPeripheralManager(delegate: self, queue: nil, options: [
                CBPeripheralManagerOptionRestoreIdentifierKey: "kinjo.ble.peripheral",
                CBPeripheralManagerOptionShowPowerAlertKey: false,
            ])
        }
    }

    @objc private func toForeground() {
        foreground = true
        if on { scan(); advertise(); reportedAt = .distantPast; step() }
    }

    @objc private func toBackground() {
        foreground = false
        if on { scan(); advertise() }
    }

    /** Every few seconds while awake, and from Bluetooth callbacks: token, advert, scan, keep-alive, report. */
    private func step() {
        guard on else { return }
        let now = Date()
        if token == nil || now >= tokenDue { fetchToken() }
        if pm?.state == .poweredOn, pm?.isAdvertising == false { advertise() }
        for (id, p) in kept where p.state != .connected { kept[id] = nil }
        reportIfDue(now)
    }

    private func reportIfDue(_ now: Date) {
        let soon = soonAt.map { now >= $0 } ?? false
        if !soon && now.timeIntervalSince(reportedAt) < (foreground ? Self.reportFgEvery : Self.reportEvery) { return }
        soonAt = nil
        reportedAt = now
        heardAt = heardAt.filter { now.timeIntervalSince($0.value) <= Self.heardFor }
        known = known.filter { now.timeIntervalSince($0.value.at) <= Self.cacheFor || kept[$0.key] != nil }
        failedAt = failedAt.filter { now.timeIntervalSince($0.value) <= Self.cacheFor }
        fails = fails.filter { failedAt[$0.key] != nil }
        lastRSSI = lastRSSI.filter { known[$0.key] != nil || connecting[$0.key] != nil }
        var seen: [[String: Any]] = direct.map { e -> [String: Any] in e.value < 0 ? ["tok": e.key, "rssi": e.value] : ["tok": e.key] }
        seen += relays.subtracting(direct.keys).map { t -> [String: Any] in ["tok": t, "via": "relay"] }
        direct = [:]; relays = []
        stride(from: 0, to: seen.count, by: Self.maxSightings).forEach {
            Presence.request("POST", "/api/sightings", ["seen": Array(seen[$0..<min($0 + Self.maxSightings, seen.count)])]) { [weak self] code, _ in
                if code == 409 { self?.tokenDue = .distantPast; self?.step() } // the server doesn't know our token: nobody can hear us
            }
        }
    }

    private func fetchToken() {
        tokenDue = Date().addingTimeInterval(60) // retry in a minute if this fails
        Presence.request("GET", "/api/ble-token", nil) { [weak self] code, data in
            guard let self = self, self.on, code == 200, let data = data,
                  let o = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let t = o["token"] as? String, Self.isToken(t) else { return }
            let every = (o["every"] as? Int) ?? 900
            self.tokenDue = Date().addingTimeInterval(TimeInterval(max(60, every - 30)))
            if t != self.token {
                self.token = t
                self.advertise() // the new name; TOKEN reads answer the new one at once
            }
        }
    }

    // MARK: being heard

    private func advertise() {
        guard on, let pm = pm, pm.state == .poweredOn, token != nil else { return }
        if !serviceAdded { addService(); return } // advertising starts in didAdd
        pm.stopAdvertising()
        var adv: [String: Any] = [CBAdvertisementDataServiceUUIDsKey: [Self.kinjo, Self.overflowMarker]] // kinjo first: it keeps the main packet
        if foreground, let t = token { adv[CBAdvertisementDataLocalNameKey] = t } // dropped by iOS in the background
        pm.startAdvertising(adv)
    }

    private func addService() {
        guard let pm = pm else { return }
        if resetServices { pm.removeAllServices(); resetServices = false }
        // values nil: answered in didReceiveRead / didReceiveWrite, so a rotated token is served at once
        let t = CBMutableCharacteristic(type: Self.tokenChar, properties: [.read], value: nil, permissions: [.readable])
        let s = CBMutableCharacteristic(type: Self.signalChar, properties: [.write, .notify], value: nil, permissions: [.writeable])
        let svc = CBMutableService(type: Self.kinjo, primary: true)
        svc.characteristics = [t, s]
        signal = s
        serviceAdded = true
        pm.add(svc)
    }

    func peripheralManagerDidUpdateState(_ peripheral: CBPeripheralManager) {
        settleAsk()
        if peripheral.state == .poweredOn { advertise() } else { serviceAdded = false; subscribers = [] } // services are dropped when Bluetooth goes off
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, willRestoreState dict: [String: Any]) {
        let services = dict[CBPeripheralManagerRestoredStateServicesKey] as? [CBMutableService] ?? []
        guard let svc = services.first(where: { $0.uuid == Self.kinjo }) else { return }
        // an older build's service has no SIGNAL: replace it
        if let s = svc.characteristics?.first(where: { $0.uuid == Self.signalChar }) as? CBMutableCharacteristic {
            signal = s
            subscribers = s.subscribedCentrals ?? []
            serviceAdded = true
        } else {
            resetServices = true
        }
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, didAdd service: CBService, error: Error?) {
        if error != nil { serviceAdded = false; return } // tried again on the next step
        advertise()
    }

    func peripheralManagerDidStartAdvertising(_ peripheral: CBPeripheralManager, error: Error?) {
        // on error isAdvertising stays false and the next step tries again
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, didReceiveRead request: CBATTRequest) {
        guard request.characteristic.uuid == Self.tokenChar, on, let t = token, let v = Self.bytes(t) else {
            peripheral.respond(to: request, withResult: .attributeNotFound)
            return
        }
        guard request.offset <= v.count else { peripheral.respond(to: request, withResult: .invalidOffset); return }
        request.value = v.subdata(in: request.offset..<v.count)
        peripheral.respond(to: request, withResult: .success)
    }

    /** A central wrote SIGNAL: 0x01 = it is near (over a connection), 0x02 = a relay. */
    func peripheralManager(_ peripheral: CBPeripheralManager, didReceiveWrite requests: [CBATTRequest]) {
        guard let first = requests.first else { return }
        var ok = true
        for r in requests {
            guard r.characteristic.uuid == Self.signalChar, let v = r.value else { ok = false; continue }
            frame(v, rssi: 0)
        }
        peripheral.respond(to: first, withResult: ok ? CBATTError.Code.success : CBATTError.Code.requestNotSupported)
        notifyAlive() // answer: keeps the writer's app awake too
        reportIfDue(Date())
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, central: CBCentral, didSubscribeTo characteristic: CBCharacteristic) {
        if characteristic.uuid == Self.signalChar, !subscribers.contains(where: { $0.identifier == central.identifier }) { subscribers.append(central) }
        notifyAlive()
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, central: CBCentral, didUnsubscribeFrom characteristic: CBCharacteristic) {
        subscribers.removeAll { $0.identifier == central.identifier }
    }

    func peripheralManagerIsReady(toUpdateSubscribers peripheral: CBPeripheralManager) {
        // a notify queue was full: the next keep-alive sends again
    }

    /** 0x01 + our token to every subscribed iPhone. */
    private func notifyAlive() {
        guard on, let pm = pm, let s = signal, !subscribers.isEmpty, let f = selfFrame() else { return }
        _ = pm.updateValue(f, for: s, onSubscribedCentrals: subscribers)
    }

    // MARK: hearing

    private func scan() {
        guard on, let cm = cm, cm.state == .poweredOn else { return }
        cm.stopScan()
        // duplicates while open (RSSI for every advert); iOS ignores the option in the background
        cm.scanForPeripherals(withServices: [Self.kinjo], options: [CBCentralManagerScanOptionAllowDuplicatesKey: foreground])
    }

    func centralManagerDidUpdateState(_ central: CBCentralManager) {
        settleAsk()
        guard central.state == .poweredOn else { connecting = [:]; kept = [:]; return }
        if on { scan() } else if Presence.session != nil, allowed { start() }
        for p in kept.values where known[p.identifier] == nil { p.discoverServices([Self.kinjo]) } // restored: read its token again
    }

    func centralManager(_ central: CBCentralManager, willRestoreState dict: [String: Any]) {
        // connections iOS kept for us while Kinjo was gone: carry on with them
        for p in dict[CBCentralManagerRestoredStatePeripheralsKey] as? [CBPeripheral] ?? [] {
            p.delegate = self
            if p.state == .connected { kept[p.identifier] = p }
        }
    }

    func centralManager(_ central: CBCentralManager, didDiscover p: CBPeripheral, advertisementData adv: [String: Any], rssi: NSNumber) {
        guard on else { return }
        let r = rssi.intValue
        let id = p.identifier
        lastRSSI[id] = r
        defer { reportIfDue(Date()) }
        if let d = (adv[CBAdvertisementDataServiceDataKey] as? [CBUUID: Data])?[Self.kinjo], d.count == 8 { // Android's own advert
            heard(Self.hex(d), r)
            return
        }
        let cached = known[id].map { Date().timeIntervalSince($0.at) <= Self.cacheFor } ?? false
        if let n = adv[CBAdvertisementDataLocalNameKey] as? String, Self.isToken(n) { heard(n, r) } // an open iPhone: connected below too, for when it goes to the background
        else if cached, let k = known[id] { heard(k.tok, r) }
        if kept[id] != nil || connecting[id] != nil || connecting.count >= Self.maxConnecting { return }
        if cached && kept.count >= Self.maxKept { return } // no room to keep it: the cache covers it
        if cached, let k = known[id], !iphones.contains(k.tok) { return } // an Android beacon: the cache covers it
        if let f = failedAt[id], Date().timeIntervalSince(f) < Self.retryWait(fails[id] ?? 1) { return }
        connecting[id] = p
        p.delegate = self
        central.connect(p, options: nil)
        DispatchQueue.main.asyncAfter(deadline: .now() + Self.connectFor) { [weak self] in
            guard let self = self, self.connecting[id] != nil else { return }
            self.fail(p) // connects never time out on iOS: give up ourselves
        }
    }

    private var iphones: Set<String> = [] // tokens whose SIGNAL notifies (iPhones): worth reconnecting to

    func centralManager(_ central: CBCentralManager, didConnect p: CBPeripheral) {
        guard on, connecting[p.identifier] != nil || kept[p.identifier] != nil else { central.cancelPeripheralConnection(p); return }
        p.discoverServices([Self.kinjo])
    }

    func centralManager(_ central: CBCentralManager, didFailToConnect p: CBPeripheral, error: Error?) {
        fail(p)
    }

    func centralManager(_ central: CBCentralManager, didDisconnectPeripheral p: CBPeripheral, error: Error?) {
        connecting[p.identifier] = nil
        kept[p.identifier] = nil
    }

    func peripheral(_ p: CBPeripheral, didDiscoverServices error: Error?) {
        guard let s = p.services?.first(where: { $0.uuid == Self.kinjo }) else { fail(p); return }
        p.discoverCharacteristics([Self.tokenChar, Self.signalChar], for: s)
    }

    func peripheral(_ p: CBPeripheral, didDiscoverCharacteristicsFor service: CBService, error: Error?) {
        guard let t = char(p, Self.tokenChar) else { fail(p); return }
        p.readValue(for: t)
    }

    func peripheral(_ p: CBPeripheral, didUpdateValueFor c: CBCharacteristic, error: Error?) {
        let id = p.identifier
        guard error == nil, let v = c.value else {
            if c.uuid == Self.tokenChar { fail(p) }
            return
        }
        if c.uuid == Self.signalChar { frame(v, rssi: lastRSSI[id] ?? 0); reportIfDue(Date()); return } // an iPhone's notify
        guard c.uuid == Self.tokenChar, v.count == 8 else { fail(p); return }
        let tok = Self.hex(v)
        connecting[id] = nil
        failedAt[id] = nil; fails[id] = nil
        known[id] = Known(tok: tok, at: known[id]?.tok == tok ? known[id]!.at : Date())
        heard(tok, lastRSSI[id] ?? 0)
        let s = char(p, Self.signalChar)
        if let s = s { write(p, s) }
        if let s = s, s.properties.contains(.notify) { iphones.insert(tok) }
        if let s = s, s.properties.contains(.notify), kept[id] != nil || kept.count < Self.maxKept { // an iPhone: stay connected
            kept[id] = p
            p.setNotifyValue(true, for: s)
            keepAlive(p)
        } else if kept[id] == nil {
            // Android (its beacon has no notify), or enough kept: the cache covers the next sightings
            DispatchQueue.main.asyncAfter(deadline: .now() + 1) { [weak self] in self?.cm?.cancelPeripheralConnection(p) } // after the write lands
        }
        reportIfDue(Date())
    }

    func peripheral(_ p: CBPeripheral, didWriteValueFor c: CBCharacteristic, error: Error?) {
        // a failed keep-alive write: the next RSSI read tries again
    }

    /** The keep-alive loop: RSSI, then a write, then again after keepEvery (driven by the callbacks). */
    func peripheral(_ p: CBPeripheral, didReadRSSI rssi: NSNumber, error: Error?) {
        let id = p.identifier
        guard on, error == nil, kept[id] != nil, let k = known[id] else { return }
        lastRSSI[id] = rssi.intValue
        heard(k.tok, rssi.intValue)
        if let s = char(p, Self.signalChar) { write(p, s) }
        notifyAlive()
        keepAlive(p)
        reportIfDue(Date())
    }

    private func keepAlive(_ p: CBPeripheral) {
        DispatchQueue.main.asyncAfter(deadline: .now() + Self.keepEvery) { [weak self] in
            guard let self = self, self.on, self.kept[p.identifier] != nil, p.state == .connected else { return }
            p.readRSSI()
        }
    }

    private func write(_ p: CBPeripheral, _ c: CBCharacteristic) {
        guard let f = selfFrame() else { return }
        p.writeValue(f, for: c, type: .withResponse)
    }

    /** A SIGNAL frame from a peer. rssi 0: heard over a connection, no RSSI. */
    private func frame(_ v: Data, rssi: Int) {
        guard v.count == 9 else { return }
        let b = [UInt8](v)
        let tok = Self.hex(Data(b[1..<9]))
        switch b[0] {
        case 0x01: heard(tok, rssi)
        case 0x02: if tok != token { relays.insert(tok) }
        default: break // a later version's frame
        }
    }

    private func heard(_ tok: String, _ rssi: Int) {
        guard tok != token else { return }
        let r = rssi < 0 && rssi != 127 ? rssi : 0 // 127: iOS has no RSSI for it
        if let prev = direct[tok] {
            if r < 0 && (prev == 0 || r > prev) { direct[tok] = r }
        } else {
            direct[tok] = r
        }
        let now = Date()
        if now.timeIntervalSince(heardAt[tok] ?? .distantPast) > Self.newAfter && soonAt == nil { // someone new: report after newDelay (batches a group arriving)
            soonAt = now.addingTimeInterval(Self.newDelay)
            DispatchQueue.main.asyncAfter(deadline: .now() + Self.newDelay) { [weak self] in self?.reportIfDue(Date()) }
        }
        heardAt[tok] = now
    }

    private func fail(_ p: CBPeripheral) {
        failedAt[p.identifier] = Date()
        fails[p.identifier, default: 0] += 1
        connecting[p.identifier] = nil
        kept[p.identifier] = nil
        cm?.cancelPeripheralConnection(p)
    }

    private func char(_ p: CBPeripheral, _ u: CBUUID) -> CBCharacteristic? {
        p.services?.first(where: { $0.uuid == Self.kinjo })?.characteristics?.first(where: { $0.uuid == u })
    }

    private func selfFrame() -> Data? {
        guard let t = token, let b = Self.bytes(t) else { return nil }
        return Data([0x01]) + b
    }

    private func settleAsk() {
        guard asked, !askWaiters.isEmpty else { return }
        let w = askWaiters
        askWaiters = []
        w.forEach { $0(allowed) }
    }

    /** A failed connect often works the next time. */
    private static func retryWait(_ fails: Int) -> TimeInterval { // 2, 5, 10, 20, 40, then 60 s
        fails <= 1 ? 2 : min(60, 5 * pow(2, Double(fails - 2)))
    }

    private static func isToken(_ s: String) -> Bool {
        s.count == 16 && s.allSatisfy { ("0"..."9").contains($0) || ("a"..."f").contains($0) }
    }

    private static func hex(_ d: Data) -> String { d.map { String(format: "%02x", $0) }.joined() }

    private static func bytes(_ s: String) -> Data? {
        guard s.count % 2 == 0 else { return nil }
        var out = Data()
        var i = s.startIndex
        while i < s.endIndex {
            let j = s.index(i, offsetBy: 2)
            guard let b = UInt8(s[i..<j], radix: 16) else { return nil }
            out.append(b)
            i = j
        }
        return out
    }
}
