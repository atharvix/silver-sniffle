import CoreLocation
import Foundation
import UIKit
import UserNotifications

/**
 * iPhone counterpart of the Android PresenceService: reports this phone's position to
 * POST /api/presence (same body: lat, lng, acc, age), with the app open or in the background.
 * Bluetooth (Ble) runs next to it, from start()/stop().
 *
 * iOS has no foreground service. What keeps Kinjo reporting:
 * - running: continuous updates with "Always" + UIBackgroundModes location keep the app alive
 *   in the background (never paused, so a still phone keeps getting fixes). In the background
 *   GPS is duty-cycled: coarse (cell/wifi, cheap) between fixes, a GPS burst every bgEvery.
 * - ended by iOS (memory): significant-change, visits and a 100 m geofence around the last fix
 *   relaunch Kinjo when the phone moves; the server's silent wake push relaunches a still one.
 * - swiped away: iOS relaunches nothing (iOS 26 no longer even honours location triggers).
 *   The Location Push Service Extension (KinjoLocationPush, needs Apple's entitlement) answers
 *   the server's location pushes with one fix each, and a local notice asks to reopen Kinjo.
 * The server forgets a fix after 2 minutes, like on Android.
 * Main thread only (CLLocationManager and the state below).
 */
@objc public final class Presence: NSObject, CLLocationManagerDelegate {
    @objc public static let shared = Presence()

    static let fgEvery: TimeInterval = 10, bgEvery: TimeInterval = 45, minEvery: TimeInterval = 5
    static let moveM: CLLocationDistance = 4 // a move this big goes out sooner (still at most every minEvery)
    static let burstFor: TimeInterval = 20, goodAcc: CLLocationAccuracy = 20 // a background GPS burst: until a fix this good, or this long
    static let fenceM: CLLocationDistance = 100, refenceM: CLLocationDistance = 50 // geofence radius; re-armed once the last fix is this far from its centre
    static let group = "group.com.kinjo.app" // shared with the KinjoLocationPush extension
    private static let apiKey = "kinjo.presence.api", tokenKey = "kinjo.presence.token", locKey = "kinjo.presence.loctoken"
    private static let fenceId = "kinjo.here"

    private let lm = CLLocationManager()
    /** The session, shared with Ble and the KinjoLocationPush extension (the app's own defaults without the app group entitlement). */
    static let store = UserDefaults(suiteName: Presence.group) ?? .standard
    private var prefs: UserDefaults { Self.store }
    private var on = false, foreground = true
    private var sentAt = Date.distantPast, sentLoc: CLLocation?
    private var burstAt: Date?, burstBest: CLLocation? // a background GPS burst in progress
    private var beat: Timer?
    private var fence: CLLocation?
    private var session: AnyObject? // CLServiceSession (iOS 18+): "Always" only counts while one is held
    private var authWaiters: [() -> Void] = []
    var onFix: (() -> Void)? // the plugin's "fix" event while the app is open

    private override init() {
        super.init()
        // builds before the app group kept the session in the app's own defaults
        if prefs.string(forKey: Self.tokenKey) == nil, let t = UserDefaults.standard.string(forKey: Self.tokenKey),
           let a = UserDefaults.standard.string(forKey: Self.apiKey) {
            prefs.set(a, forKey: Self.apiKey); prefs.set(t, forKey: Self.tokenKey)
        }
        lm.delegate = self
        lm.pausesLocationUpdatesAutomatically = false // iOS would otherwise stop updates for a still phone, for good
        lm.activityType = .other
        let nc = NotificationCenter.default
        nc.addObserver(self, selector: #selector(toForeground), name: UIApplication.didBecomeActiveNotification, object: nil)
        nc.addObserver(self, selector: #selector(toBackground), name: UIApplication.didEnterBackgroundNotification, object: nil)
    }

    var auth: CLAuthorizationStatus { lm.authorizationStatus }
    var located: Bool { auth == .authorizedAlways || auth == .authorizedWhenInUse }
    var always: Bool { auth == .authorizedAlways }

    /** Remember the session; nil forgets it (hidden, signed out, account deleted). */
    func save(api: String?, token: String?) {
        if let api = api, let token = token {
            if token != prefs.string(forKey: Self.tokenKey) { prefs.removeObject(forKey: Self.locKey) } // new session: register the location-push token again
            prefs.set(api, forKey: Self.apiKey); prefs.set(token, forKey: Self.tokenKey)
        } else {
            [Self.apiKey, Self.tokenKey, Self.locKey].forEach { prefs.removeObject(forKey: $0) }
        }
    }

    /** Relaunched by iOS (location event, wake push) or opened: carry on if signed in. */
    @objc public func resumeIfSaved() {
        if prefs.string(forKey: Self.tokenKey) != nil { start() }
    }

    func start() {
        Ble.shared.start() // on its own: no location needed, no-op until Bluetooth is allowed
        guard located, prefs.string(forKey: Self.tokenKey) != nil else { return }
        foreground = UIApplication.shared.applicationState != .background
        hold()
        tune()
        if !on {
            on = true
            lm.startUpdatingLocation() // from a background relaunch this needs "Always": with it, the app keeps running
            beat?.invalidate()
            beat = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in self?.tick() }
        }
        if always {
            lm.startMonitoringSignificantLocationChanges() // relaunches Kinjo after iOS ends it (500 m moves)
            lm.startMonitoringVisits() // and on arriving at / leaving a place
            registerLocationPushes()
        }
    }

    func stop() {
        on = false
        Ble.shared.stop()
        beat?.invalidate(); beat = nil
        burstAt = nil; burstBest = nil
        lm.stopUpdatingLocation()
        lm.stopMonitoringSignificantLocationChanges()
        lm.stopMonitoringVisits()
        lm.monitoredRegions.filter { $0.identifier == Self.fenceId }.forEach { lm.stopMonitoring(for: $0) }
        fence = nil
        lm.stopMonitoringLocationPushes()
        lm.allowsBackgroundLocationUpdates = false
        session = nil
        save(api: nil, token: nil)
    }

    /** iOS 18+: hold an "Always" service session while visible (location pushes and relaunches need it). Retaken on every launch. */
    private func hold() {
        guard always, session == nil else { return }
        if #available(iOS 18.0, *) { session = CLServiceSession(authorization: .always) }
    }

    /**
     * On screen: best accuracy, every fix. Background: coarse between fixes (cell/wifi, GPS off;
     * the updates keep running, so iOS keeps Kinjo running) and GPS only during a burst.
     */
    private func tune() {
        lm.desiredAccuracy = foreground ? kCLLocationAccuracyBest
            : burstAt != nil ? kCLLocationAccuracyNearestTenMeters : kCLLocationAccuracyThreeKilometers
        lm.distanceFilter = kCLDistanceFilterNone // a still phone still needs fresh fixes: the server forgets after 2 min
        lm.allowsBackgroundLocationUpdates = located // needs UIBackgroundModes: location
        lm.showsBackgroundLocationIndicator = false
    }

    @objc private func toForeground() {
        foreground = true
        burstAt = nil; burstBest = nil
        if on { tune(); sentAt = .distantPast } // the next fix goes out at once
    }

    @objc private func toBackground() {
        foreground = false
        if on { tune() }
    }

    /** Every 5 s while running: start a background burst when a fix is due, end one that ran out of time. */
    private func tick() {
        guard on, !foreground else { return }
        if let b = burstAt {
            if Date().timeIntervalSince(b) >= Self.burstFor { endBurst(send: burstBest) }
        } else if Date().timeIntervalSince(sentAt) >= Self.bgEvery {
            burst()
        }
    }

    private func burst() {
        guard on, !foreground, burstAt == nil else { return }
        burstAt = Date(); burstBest = nil
        tune()
    }

    private func endBurst(send l: CLLocation?) {
        burstAt = nil; burstBest = nil
        if let l = l { send(l) } else { sentAt = Date().addingTimeInterval(Self.minEvery * 2 - Self.bgEvery) } // no fix: try again soon
        tune()
    }

    /** The server's silent wake push: get a fix out now. done: when iOS may suspend us again. */
    @objc public func poke(_ done: @escaping () -> Void) {
        start()
        Ble.shared.kick()
        guard on else { done(); return }
        sentAt = .distantPast
        if !foreground { burst() }
        DispatchQueue.main.asyncAfter(deadline: .now() + 20) { done() } // iOS gives a wake push ~30 s
    }

    /** The app is being closed while visible (swiped away, the only case iOS calls this for): say so, nothing restarts it. */
    @objc public func terminating() {
        guard on, prefs.string(forKey: Self.tokenKey) != nil else { return }
        let c = UNMutableNotificationContent()
        c.title = "Kinjo is closed"
        c.body = "People near you can't see your card. Open Kinjo to stay visible."
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: "kinjo.closed", content: c, trigger: nil))
    }

    public func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let l = locations.last else { return }
        onFix?()
        guard on else { return }
        if foreground {
            let since = Date().timeIntervalSince(sentAt)
            let moved = sentLoc.map { l.distance(from: $0) } ?? .greatestFiniteMagnitude
            if since < Self.minEvery || (moved < Self.moveM && since < Self.fgEvery) { return }
            send(l)
            return
        }
        guard let b = burstAt else {
            // coarse fix between bursts: only a sign of movement
            if let s = sentLoc, l.distance(from: s) > max(Self.refenceM, l.horizontalAccuracy), Date().timeIntervalSince(sentAt) >= Self.minEvery { burst() }
            return
        }
        guard l.timestamp >= b.addingTimeInterval(-1), l.horizontalAccuracy >= 0 else { return } // a cached fix from before the burst
        if burstBest.map({ l.horizontalAccuracy <= $0.horizontalAccuracy }) ?? true { burstBest = l }
        if l.horizontalAccuracy <= Self.goodAcc { endBurst(send: l) }
    }

    public func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        // no fix right now (indoors, airplane mode): updates keep coming once there is one
    }

    /** Left the geofence (also relaunches Kinjo after iOS ended it): a fix now, the fence moves with it. */
    public func locationManager(_ manager: CLLocationManager, didExitRegion region: CLRegion) {
        guard region.identifier == Self.fenceId else { return }
        fence = nil
        start()
        sentAt = .distantPast
        burst()
    }

    public func locationManager(_ manager: CLLocationManager, monitoringDidFailFor region: CLRegion?, withError error: Error) {
        if region?.identifier == Self.fenceId { fence = nil } // re-armed on the next fix
    }

    public func locationManager(_ manager: CLLocationManager, didVisit visit: CLVisit) {
        start() // relaunched on arriving or leaving: updates take over
    }

    public func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        if located { resumeIfSaved() } else if on {
            on = false
            beat?.invalidate(); beat = nil
            lm.stopUpdatingLocation(); lm.stopMonitoringSignificantLocationChanges(); lm.stopMonitoringVisits()
        }
        if !always { session = nil }
        if auth == .notDetermined { return } // the first call comes when the manager is made, not from a choice
        let w = authWaiters
        authWaiters = []
        w.forEach { $0() }
    }

    /** Android-style "while using" prompt; done once the person chose (at once if they already did). */
    func askWhenInUse(_ done: @escaping () -> Void) {
        guard auth == .notDetermined else { done(); return }
        authWaiters.append(done)
        lm.requestWhenInUseAuthorization()
    }

    /**
     * Ask for "Always". iOS shows that choice only once; after that it lives in Settings,
     * so when no prompt appears Kinjo's settings page opens instead (like Android 11+).
     */
    func askAlways(_ done: @escaping (Bool) -> Void) {
        if always { done(true); return }
        guard auth == .authorizedWhenInUse else { done(false); return }
        let nc = NotificationCenter.default
        var observers: [NSObjectProtocol] = []
        var prompted = false, finished = false
        let finish: () -> Void = {
            if finished { return }
            finished = true
            observers.forEach { nc.removeObserver($0) }
            done(self.always)
        }
        observers.append(nc.addObserver(forName: UIApplication.willResignActiveNotification, object: nil, queue: .main) { _ in prompted = true })
        observers.append(nc.addObserver(forName: UIApplication.didBecomeActiveNotification, object: nil, queue: .main) { _ in if prompted { finish() } })
        lm.requestAlwaysAuthorization()
        DispatchQueue.main.asyncAfter(deadline: .now() + 1) {
            if !prompted { Presence.openSettings(); finish() }
        }
    }

    static func openSettings() {
        if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
    }

    /** Moves the geofence to the last fix once it drifted refenceM from its centre. Exiting it relaunches Kinjo. */
    private func refence(_ l: CLLocation) {
        guard always, CLLocationManager.isMonitoringAvailable(for: CLCircularRegion.self),
              l.horizontalAccuracy >= 0, l.horizontalAccuracy <= Self.fenceM else { return }
        if let f = fence, l.distance(from: f) < Self.refenceM { return }
        let r = CLCircularRegion(center: l.coordinate, radius: Self.fenceM, identifier: Self.fenceId) // same id replaces the old one
        r.notifyOnEntry = false
        r.notifyOnExit = true
        lm.startMonitoring(for: r)
        fence = l
    }

    /**
     * The KinjoLocationPush extension's APNs token (needs the com.apple.developer.location.push
     * entitlement: without it this fails and nothing else changes). Sent once per session;
     * the server sends location pushes to it when this phone goes quiet.
     */
    private func registerLocationPushes() {
        lm.startMonitoringLocationPushes { [weak self] token, _ in
            guard let self = self, let token = token else { return }
            let hex = token.map { String(format: "%02x", $0) }.joined()
            DispatchQueue.main.async {
                guard self.on, self.prefs.string(forKey: Self.locKey) != hex else { return }
                self.post("/api/device", ["pushToken": hex, "platform": "ioslocation"]) { ok in
                    if ok { self.prefs.set(hex, forKey: Self.locKey) }
                }
            }
        }
    }

    private func send(_ l: CLLocation) {
        sentAt = Date()
        sentLoc = l
        refence(l)
        let acc = l.horizontalAccuracy >= 0 ? max(1, Int(l.horizontalAccuracy.rounded())) : 0 // 0 = unknown: the server won't match on it
        let age = max(0, Int(Date().timeIntervalSince(l.timestamp) * 1000)) // the server dates the fix by when it was taken
        let r6 = { (v: Double) in (v * 1e6).rounded() / 1e6 }
        post("/api/presence", ["lat": r6(l.coordinate.latitude), "lng": r6(l.coordinate.longitude), "acc": acc, "age": age])
    }

    private func post(_ path: String, _ body: [String: Any], _ then: ((Bool) -> Void)? = nil) {
        guard let api = prefs.string(forKey: Self.apiKey), let token = prefs.string(forKey: Self.tokenKey),
              let url = URL(string: api + path) else { return }
        var r = URLRequest(url: url, timeoutInterval: 10)
        r.httpMethod = "POST"
        r.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
        r.setValue("application/json", forHTTPHeaderField: "Content-Type")
        r.httpBody = try? JSONSerialization.data(withJSONObject: body)
        // a few seconds of runtime so a post started just before suspension still lands
        var task = UIBackgroundTaskIdentifier.invalid
        let end: () -> Void = {
            if task != .invalid { UIApplication.shared.endBackgroundTask(task); task = .invalid }
        }
        task = UIApplication.shared.beginBackgroundTask(withName: "kinjo.presence", expirationHandler: end)
        URLSession.shared.dataTask(with: r) { _, resp, _ in
            DispatchQueue.main.async {
                let code = (resp as? HTTPURLResponse)?.statusCode ?? 0
                if code == 401 { self.stop() } // session gone: stop for good
                then?((200..<300).contains(code))
                end()
            }
        }.resume()
    }
}
