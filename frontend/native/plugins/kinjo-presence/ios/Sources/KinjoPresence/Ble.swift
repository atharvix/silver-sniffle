import CoreBluetooth
import Foundation
import UIKit

/**
 * Bluetooth on iPhone, the counterpart of the Android Ble: hear Kinjo phones nearby and be
 * heard, reporting tokens to POST /api/sightings exactly like Android (one side hearing
 * counts for both, server side).
 *
 * iOS can't advertise service data, and in the background its service UUIDs move to Apple's
 * "overflow" area (only iPhones scanning for that UUID see them; Android can't). So:
 * - advertise KINJO (+ the token as local name while open: open iPhones read it without connecting)
 *   and serve the token as a readable GATT characteristic (TOKEN_CHAR) for everyone else;
 * - scan for KINJO (a background scan must name it) and get a peer's token from, in order:
 *   Android service data, the local name, a cached read, else connect and read TOKEN_CHAR.
 *   New Android builds add a connectable advert listing KINJO with the same characteristic,
 *   which is what a background iPhone can see. Old Android builds are heard only while open.
 * - stay connected to background iPhones (few) and read RSSI each step: iOS reports a
 *   background advertiser once, then coalesces, so the connection is what keeps the pair fresh.
 *
 * Main thread only. Starts only once Bluetooth is allowed (creating a manager shows iOS's prompt).
 */
final class Ble: NSObject, CBCentralManagerDelegate, CBPeripheralManagerDelegate, CBPeripheralDelegate {
    static let shared = Ble()

    static let kinjo = CBUUID(string: "6b1a0c30-4f9e-4b8e-9c55-2d3c7e1f0a30")
    static let tokenChar = CBUUID(string: "6b1a0c31-4f9e-4b8e-9c55-2d3c7e1f0a30") // must match Android's Ble.TOKEN_CHAR
    static let reportEvery: TimeInterval = 10, reportFgEvery: TimeInterval = 5, heardFor: TimeInterval = 120
    static let newAfter: TimeInterval = 20 // not heard this long: report at once (the server's pair window is 30 s)
    static let cacheFor: TimeInterval = 600 // a peer's token, by peripheral id (tokens rotate every 15 min; the server honours the last one 15 min more)
    static let connectFor: TimeInterval = 8, retryAfter: TimeInterval = 60
    static let maxConnecting = 3, maxKept = 6, maxSightings = 32, minRSSI = -95
    private static let apiKey = "kinjo.presence.api", tokenKey = "kinjo.presence.token"

    private struct Known { var tok: String; var at: Date }

    private var cm: CBCentralManager?
    private var pm: CBPeripheralManager?
    private var prefs: UserDefaults { Presence.store } // the session Presence saved
    private var on = false, foreground = true, serviceAdded = false
    private var token: String?, tokenDue = Date.distantPast
    private var timer: Timer?, reportedAt = Date.distantPast
    private var batch: [String: Int] = [:] // token -> strongest RSSI since the last report
    private var heardAt: [String: Date] = [:]
    private var known: [UUID: Known] = [:] // peripheral -> its token
    private var connecting: [UUID: CBPeripheral] = [:] // connect started, token not read yet
    private var kept: [UUID: CBPeripheral] = [:] // background iPhones we stay connected to
    private var keep: Set<UUID> = [] // to keep once read
    private var lastRSSI: [UUID: Int] = [:]
    private var failedAt: [UUID: Date] = [:]
    private var askWaiters: [(Bool) -> Void] = []

    var allowed: Bool { CBManager.authorization == .allowedAlways }
    var asked: Bool { CBManager.authorization != .notDetermined }
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

    /** Called from Presence.start (also on a background relaunch): no-op until allowed and signed in. */
    func start() {
        guard allowed, prefs.string(forKey: Self.tokenKey) != nil else { return }
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
        timer?.invalidate()
        timer = nil
        cm?.stopScan()
        pm?.stopAdvertising()
        for p in Array(connecting.values) + Array(kept.values) { cm?.cancelPeripheralConnection(p) }
        connecting = [:]; kept = [:]; keep = []; known = [:]; batch = [:]; heardAt = [:]; lastRSSI = [:]
        token = nil
        tokenDue = .distantPast
    }

    /** The server's wake push: refresh the token if due, re-advertise, report now. */
    func kick() {
        if on { reportedAt = .distantPast; step() }
    }

    /** Same restore ids every launch: iOS hands back scans, connections and adverts after it ended Kinjo (not after a swipe-away). */
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
        if on { scan(); advertise() } // the local name is dropped by iOS anyway; duplicates are off
    }

    /** Every few seconds (and on any Bluetooth event that wakes us): token, advert, scan, keep-alive reads, report. */
    private func step() {
        guard on else { return }
        let now = Date()
        if token == nil || now >= tokenDue { fetchToken() }
        if pm?.state == .poweredOn, pm?.isAdvertising == false { advertise() }
        if cm?.state == .poweredOn, !foreground { scan() } // restart: a background scan reports each advertiser once
        for (id, p) in kept {
            guard p.state == .connected else { kept[id] = nil; continue }
            p.readRSSI()
            if let c = tokenCharacteristic(p) { p.readValue(for: c) } else { p.discoverServices([Self.kinjo]) } // restored without services
        }
        reportIfDue(now)
    }

    private func reportIfDue(_ now: Date) {
        if now.timeIntervalSince(reportedAt) < (foreground ? Self.reportFgEvery : Self.reportEvery) { return }
        reportedAt = now
        heardAt = heardAt.filter { now.timeIntervalSince($0.value) <= Self.heardFor }
        known = known.filter { now.timeIntervalSince($0.value.at) <= Self.cacheFor || kept[$0.key] != nil }
        failedAt = failedAt.filter { now.timeIntervalSince($0.value) <= Self.retryAfter }
        lastRSSI = lastRSSI.filter { known[$0.key] != nil || connecting[$0.key] != nil }
        let seen = batch.map { ["tok": $0.key, "rssi": $0.value] as [String: Any] }
        batch = [:]
        // 32 per request (the server's limit), every token heard, like Android
        stride(from: 0, to: seen.count, by: Self.maxSightings).forEach {
            request("POST", "/api/sightings", ["seen": Array(seen[$0..<min($0 + Self.maxSightings, seen.count)])])
        }
    }

    private func fetchToken() {
        tokenDue = Date().addingTimeInterval(60) // retry in a minute if this fails
        request("GET", "/api/ble-token", nil) { [weak self] data in
            guard let self = self, self.on, let data = data,
                  let o = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let t = o["token"] as? String, Self.isToken(t) else { return }
            let every = (o["every"] as? Int) ?? 900
            self.tokenDue = Date().addingTimeInterval(TimeInterval(max(60, every - 30)))
            if t != self.token {
                self.token = t
                self.advertise() // re-advertised with the new name; GATT reads answer the new token at once
            }
        }
    }

    // MARK: being heard

    private func advertise() {
        guard on, let pm = pm, pm.state == .poweredOn, let t = token else { return }
        if !serviceAdded { addService(); return } // advertising starts in didAdd
        pm.stopAdvertising()
        var adv: [String: Any] = [CBAdvertisementDataServiceUUIDsKey: [Self.kinjo]]
        if foreground { adv[CBAdvertisementDataLocalNameKey] = t } // ignored by iOS in the background
        pm.startAdvertising(adv)
    }

    private func addService() {
        guard let pm = pm else { return }
        // value nil: answered in didReceiveRead, so a rotated token is served at once
        let c = CBMutableCharacteristic(type: Self.tokenChar, properties: [.read], value: nil, permissions: [.readable])
        let s = CBMutableService(type: Self.kinjo, primary: true)
        s.characteristics = [c]
        serviceAdded = true
        pm.add(s)
    }

    func peripheralManagerDidUpdateState(_ peripheral: CBPeripheralManager) {
        settleAsk()
        if peripheral.state == .poweredOn { advertise() }
        else { serviceAdded = false } // services are dropped when Bluetooth goes off
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, willRestoreState dict: [String: Any]) {
        let services = dict[CBPeripheralManagerRestoredStateServicesKey] as? [CBMutableService] ?? []
        serviceAdded = services.contains { $0.uuid == Self.kinjo } // never add it twice
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

    // MARK: hearing

    private func scan() {
        guard on, let cm = cm, cm.state == .poweredOn else { return }
        cm.stopScan()
        // duplicates while open (RSSI for every advert); iOS ignores the option in the background
        cm.scanForPeripherals(withServices: [Self.kinjo], options: [CBCentralManagerScanOptionAllowDuplicatesKey: foreground])
    }

    func centralManagerDidUpdateState(_ central: CBCentralManager) {
        settleAsk()
        guard central.state == .poweredOn else { connecting = [:]; kept = [:]; return } // off: GPS carries on alone
        if on { scan() } else if prefs.string(forKey: Self.tokenKey) != nil, allowed { start() }
    }

    func centralManager(_ central: CBCentralManager, willRestoreState dict: [String: Any]) {
        // connections iOS kept for us while Kinjo was gone: read them again once on
        for p in dict[CBCentralManagerRestoredStatePeripheralsKey] as? [CBPeripheral] ?? [] {
            p.delegate = self
            if p.state == .connected { kept[p.identifier] = p }
        }
        // called while the manager is made in start() (AppDelegate -> Presence.resumeIfSaved on relaunch): nothing more to do
    }

    func centralManager(_ central: CBCentralManager, didDiscover p: CBPeripheral, advertisementData adv: [String: Any], rssi: NSNumber) {
        guard on else { return }
        let r = rssi.intValue
        let id = p.identifier
        lastRSSI[id] = r
        defer { reportIfDue(Date()) }
        if let d = (adv[CBAdvertisementDataServiceDataKey] as? [CBUUID: Data])?[Self.kinjo], d.count == 8 { // Android's own advert
            heard(Self.hex(d), r, id)
            return
        }
        if let n = adv[CBAdvertisementDataLocalNameKey] as? String, Self.isToken(n) { // an open iPhone
            heard(n, r, id)
            return
        }
        if let k = known[id], Date().timeIntervalSince(k.at) <= Self.cacheFor { heard(k.tok, r, id); return }
        if kept[id] != nil || connecting[id] != nil || connecting.count >= Self.maxConnecting { return }
        if let f = failedAt[id], Date().timeIntervalSince(f) < Self.retryAfter { return }
        // only overflow (a background iPhone) is worth a lasting connection; Android's advert lists KINJO normally
        let overflow = (adv[CBAdvertisementDataOverflowServiceUUIDsKey] as? [CBUUID])?.contains(Self.kinjo) ?? false
        if overflow { keep.insert(id) } else { keep.remove(id) }
        connecting[id] = p
        p.delegate = self
        central.connect(p, options: nil)
        DispatchQueue.main.asyncAfter(deadline: .now() + Self.connectFor) { [weak self] in
            guard let self = self, self.connecting[id] != nil else { return }
            self.fail(p) // connects never time out on iOS: give up ourselves
        }
    }

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
        p.discoverCharacteristics([Self.tokenChar], for: s)
    }

    func peripheral(_ p: CBPeripheral, didDiscoverCharacteristicsFor service: CBService, error: Error?) {
        guard let c = tokenCharacteristic(p) else { fail(p); return }
        p.readValue(for: c)
    }

    func peripheral(_ p: CBPeripheral, didUpdateValueFor c: CBCharacteristic, error: Error?) {
        let id = p.identifier
        guard c.uuid == Self.tokenChar, error == nil, let v = c.value, v.count == 8 else { fail(p); return }
        connecting[id] = nil
        failedAt[id] = nil
        if let r = lastRSSI[id] { heard(Self.hex(v), r, id) } else { known[id] = Known(tok: Self.hex(v), at: Date()) }
        if on, keep.contains(id), kept[id] != nil || kept.count < Self.maxKept { kept[id] = p }
        else if kept[id] == nil { cm?.cancelPeripheralConnection(p) } // Android, or enough kept: the cache covers the next sightings
        reportIfDue(Date())
    }

    func peripheral(_ p: CBPeripheral, didReadRSSI rssi: NSNumber, error: Error?) {
        guard error == nil, let k = known[p.identifier] else { return }
        lastRSSI[p.identifier] = rssi.intValue
        heard(k.tok, rssi.intValue, p.identifier)
        reportIfDue(Date())
    }

    private func heard(_ tok: String, _ rssi: Int, _ id: UUID) {
        known[id] = Known(tok: tok, at: known[id]?.tok == tok ? known[id]!.at : Date())
        guard tok != token, rssi < 0, rssi != 127 else { return } // 127: iOS has no RSSI for it
        if rssi > batch[tok] ?? Int.min { batch[tok] = rssi }
        let now = Date()
        if now.timeIntervalSince(heardAt[tok] ?? .distantPast) > Self.newAfter { reportedAt = .distantPast } // someone new: the caller's reportIfDue sends it now
        heardAt[tok] = now
    }

    private func fail(_ p: CBPeripheral) {
        failedAt[p.identifier] = Date()
        connecting[p.identifier] = nil
        kept[p.identifier] = nil
        cm?.cancelPeripheralConnection(p)
    }

    private func tokenCharacteristic(_ p: CBPeripheral) -> CBCharacteristic? {
        p.services?.first(where: { $0.uuid == Self.kinjo })?.characteristics?.first(where: { $0.uuid == Self.tokenChar })
    }

    private func settleAsk() {
        guard asked, !askWaiters.isEmpty else { return }
        let w = askWaiters
        askWaiters = []
        w.forEach { $0(allowed) }
    }

    // MARK: server

    private func request(_ method: String, _ path: String, _ body: [String: Any]?, _ done: ((Data?) -> Void)? = nil) {
        guard let api = prefs.string(forKey: Self.apiKey), let tok = prefs.string(forKey: Self.tokenKey),
              let url = URL(string: api + path) else { return }
        var r = URLRequest(url: url, timeoutInterval: 10)
        r.httpMethod = method
        r.setValue("Bearer " + tok, forHTTPHeaderField: "Authorization")
        if let body = body {
            r.setValue("application/json", forHTTPHeaderField: "Content-Type")
            r.httpBody = try? JSONSerialization.data(withJSONObject: body)
        }
        var task = UIBackgroundTaskIdentifier.invalid
        let end: () -> Void = {
            if task != .invalid { UIApplication.shared.endBackgroundTask(task); task = .invalid }
        }
        task = UIApplication.shared.beginBackgroundTask(withName: "kinjo.ble", expirationHandler: end)
        URLSession.shared.dataTask(with: r) { data, resp, _ in
            DispatchQueue.main.async {
                let code = (resp as? HTTPURLResponse)?.statusCode ?? -1
                if code == 401 { Presence.shared.stop() } // session gone: stop for good (stops Bluetooth too)
                else if code == 200 { done?(data) }
                end()
            }
        }.resume()
    }

    private static func isToken(_ s: String) -> Bool {
        s.count == 16 && s.allSatisfy { ("0"..."9").contains($0) || ("a"..."f").contains($0) }
    }

    private static func hex(_ d: Data) -> String { d.map { String(format: "%02x", $0) }.joined() }

    private static func bytes(_ s: String) -> Data? {
        var d = Data(capacity: s.count / 2)
        var i = s.startIndex
        while i < s.endIndex {
            let j = s.index(i, offsetBy: 2)
            guard let b = UInt8(s[i..<j], radix: 16) else { return nil }
            d.append(b)
            i = j
        }
        return d
    }
}
