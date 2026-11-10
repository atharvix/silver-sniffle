import CoreLocation
import CoreMotion
import Foundation
import UIKit
import UserNotifications

/**
 * The signed-in session and location, the main signal (v3, docs/proximity.md). Started from the
 * open app ("While Using"), location keeps running in the background (location background mode,
 * blue indicator), which is also what keeps Kinjo running locked. Effort follows the server's
 * answer to each report (mode, next):
 *  - with (someone in range), near (someone within ~150-450 m), or the app on screen: GPS, a
 *    report every 5 m moved;
 *  - around (~1 km): Wi-Fi/cell, every 25 m; idle (nobody): Wi-Fi/cell, every 100 m;
 *  - still (2 min within 25 m): coarse updates only (stopping them would let iOS suspend us),
 *    the last precise fix re-sent with still:true every 2 min, and the accelerometer (no
 *    permission) brings GPS back the moment the phone moves.
 * Reports go over the reporter socket (Live), HTTP when it's down. Swiped away, Kinjo can't
 * run: the KinjoLocationPush extension (needs Apple's location-push entitlement and "Always")
 * answers the server's location pushes with a fix instead. Bluetooth (Ble) runs too, if
 * allowed, for older app builds. Main thread only.
 */
@objc public final class Presence: NSObject, CLLocationManagerDelegate {
    @objc public static let shared = Presence()

    static let stillAfter: TimeInterval = 120, stillM: Double = 25, stillBeat: TimeInterval = 120, minGap: TimeInterval = 1
    static let group = "group.com.kinjo.app"
    private static let apiKey = "kinjo.presence.api", tokenKey = "kinjo.presence.token"
    private static let locKey = "kinjo.presence.loctoken", alwaysKey = "kinjo.presence.askedAlways"

    /** The session (the app group's defaults when the entitlement is there, else the app's own). */
    static let store = UserDefaults(suiteName: Presence.group) ?? .standard
    private var prefs: UserDefaults { Self.store }
    private let lm = CLLocationManager()
    private var on = false, foreground = true, updating = false
    private var sentAt = Date.distantPast
    private var mode = "near", next: TimeInterval = 15
    private var still = false, settling = false
    private var last: CLLocation?, sentFix: CLLocation?, anchor: CLLocation?, anchorAt = Date()
    private var beat: Timer?
    private let motion = CMMotionManager()
    private var shakes: [Double] = []
    private let live = Live()
    private var authWaiters: [() -> Void] = []
    var onFix: (() -> Void)? // the plugin's "fix" event

    private override init() {
        super.init()
        // builds before the app group kept the session in the app's own defaults
        if prefs.string(forKey: Self.tokenKey) == nil, let t = UserDefaults.standard.string(forKey: Self.tokenKey),
           let a = UserDefaults.standard.string(forKey: Self.apiKey) {
            prefs.set(a, forKey: Self.apiKey); prefs.set(t, forKey: Self.tokenKey)
        }
        lm.delegate = self
        lm.desiredAccuracy = kCLLocationAccuracyBest
        lm.distanceFilter = kCLDistanceFilterNone // iOS 16.4+ may stop background updates with a filter set: we filter ourselves
        lm.pausesLocationUpdatesAutomatically = false // or iOS pauses a still phone after ~10 min and never resumes in the background
        lm.activityType = .other
        live.onMode = { [weak self] m, n in self?.server(m, n) }
        live.onHidden = { [weak self] in self?.hiddenElsewhere() }
        live.onOpen = { [weak self] in self?.resend() }
        let nc = NotificationCenter.default
        nc.addObserver(self, selector: #selector(toForeground), name: UIApplication.didBecomeActiveNotification, object: nil)
        nc.addObserver(self, selector: #selector(toBackground), name: UIApplication.didEnterBackgroundNotification, object: nil)
    }

    var auth: CLAuthorizationStatus { lm.authorizationStatus }
    var located: Bool { auth == .authorizedAlways || auth == .authorizedWhenInUse }
    var running: Bool { on }

    static var session: (api: String, token: String)? {
        guard let a = store.string(forKey: apiKey), let t = store.string(forKey: tokenKey) else { return nil }
        return (a, t)
    }

    /** Remember the session; nil forgets it (hidden, signed out, account deleted). */
    func save(api: String?, token: String?) {
        if let api = api, let token = token {
            prefs.set(api, forKey: Self.apiKey); prefs.set(token, forKey: Self.tokenKey)
        } else {
            [Self.apiKey, Self.tokenKey].forEach { prefs.removeObject(forKey: $0) }
        }
    }

    /** Launched (also relaunched by iOS for Bluetooth or a wake push): carry on if signed in. */
    @objc public func resumeIfSaved() {
        if Self.session != nil { start() }
    }

    func start() {
        guard Self.session != nil else { return }
        on = true
        live.reset()
        foreground = UIApplication.shared.applicationState != .background
        Ble.shared.start() // no-op until Bluetooth is allowed
        follow()
    }

    func stop() {
        on = false
        Ble.shared.stop()
        live.stop()
        follow()
        if #available(iOS 15.0, *) { lm.stopMonitoringLocationPushes() } // signed out / hidden: no more fixes from the extension
        prefs.removeObject(forKey: Self.locKey)
        save(api: nil, token: nil)
    }

    /** Hidden on another device ("vis"): stop sharing here; the app starts us again when shown. */
    private func hiddenElsewhere() {
        on = false
        Ble.shared.stop()
        live.stop()
        follow()
    }

    /** Location updates while signed in and allowed: started on screen (a "While Using" start
     *  from the background does nothing), then kept running in the background. */
    private func follow() {
        let want = on && located && (foreground || updating)
        if want == updating { if want { tune() }; return }
        updating = want
        if want {
            if Self.backgroundLocation {
                lm.allowsBackgroundLocationUpdates = true
                lm.showsBackgroundLocationIndicator = true
            }
            still = false
            anchor = nil
            sentAt = .distantPast
            tune()
            lm.startUpdatingLocation()
            live.connect()
            registerLocationPushes()
        } else {
            lm.stopUpdatingLocation()
            beat?.invalidate()
            stopShakes()
        }
    }

    /**
     * The extension's APNs token (fails, changing nothing, without the location-push entitlement).
     * Sent once per token; then "Always" is asked once, which iOS needs to wake the extension.
     */
    private func registerLocationPushes() {
        guard #available(iOS 15.0, *) else { return }
        lm.startMonitoringLocationPushes { [weak self] token, _ in
            guard let token = token else { return }
            let hex = token.map { String(format: "%02x", $0) }.joined()
            DispatchQueue.main.async {
                guard let self = self, self.on else { return }
                if self.prefs.string(forKey: Self.locKey) != hex {
                    Self.request("POST", "/api/device", ["pushToken": hex, "platform": "ioslocation"]) { code, _ in
                        if code < 300 { self.prefs.set(hex, forKey: Self.locKey) }
                    }
                }
                if self.auth == .authorizedWhenInUse, self.foreground, !self.prefs.bool(forKey: Self.alwaysKey) {
                    self.prefs.set(true, forKey: Self.alwaysKey)
                    self.lm.requestAlwaysAuthorization() // "Change to Always Allow": found even after closing Kinjo
                }
            }
        }
    }

    /** Without the location background mode in Info.plist, setting allowsBackgroundLocationUpdates crashes. */
    private static let backgroundLocation = (Bundle.main.object(forInfoDictionaryKey: "UIBackgroundModes") as? [String])?.contains("location") ?? false

    /** GPS in with/near or on screen; Wi-Fi/cell otherwise and while still. */
    private var precise: Bool { foreground || mode == "with" || mode == "near" }

    private func tune() {
        lm.desiredAccuracy = (settling || (!still && precise)) ? kCLLocationAccuracyBest : kCLLocationAccuracyHundredMeters
    }

    /** Metres moved before a report goes out. */
    private var minMove: Double { precise ? 5 : mode == "around" ? 25 : 100 }

    @objc private func toForeground() { foreground = true; if still { wake() } else { follow() } }
    @objc private func toBackground() { foreground = false; follow() }

    /** The server's answer to a report. */
    private func server(_ m: String, _ n: Int) {
        mode = m
        next = TimeInterval(max(1, n))
        if updating { tune() }
        arm()
    }

    private func arm() {
        beat?.invalidate()
        beat = Timer.scheduledTimer(withTimeInterval: still ? Self.stillBeat : next, repeats: false) { [weak self] _ in
            guard let self = self, let l = self.last else { return }
            self.send(l)
        }
    }

    private func resend() {
        guard updating, let l = last else { return }
        sentAt = .distantPast
        send(l)
    }

    /** Stopped moving: rest on the last precise fix (asking for one if it isn't). */
    private func settle() {
        guard let l = last else { return }
        if l.horizontalAccuracy > 30 && !settling {
            settling = true
            tune()
            DispatchQueue.main.asyncAfter(deadline: .now() + 10) { [weak self] in
                guard let self = self, self.settling else { return }
                self.settling = false
                self.rest()
            }
            return
        }
        settling = false
        rest()
    }

    private func rest() {
        guard updating, !still else { return }
        still = true
        tune()
        startShakes()
        if let l = last { sentAt = .distantPast; send(l) }
    }

    /** Moving again: GPS back (in the current mode) and a report on the next fix. */
    private func wake() {
        stopShakes()
        still = false
        anchor = nil
        sentAt = .distantPast
        tune()
        follow()
    }

    /** Movement from the accelerometer, 5 times a second: a phone being carried shakes. */
    private func startShakes() {
        guard motion.isAccelerometerAvailable, !motion.isAccelerometerActive else { return }
        shakes = []
        motion.accelerometerUpdateInterval = 0.2
        motion.startAccelerometerUpdates(to: .main) { [weak self] data, _ in
            guard let self = self, let a = data?.acceleration else { return }
            self.shakes.append(sqrt(a.x * a.x + a.y * a.y + a.z * a.z))
            if self.shakes.count < 10 { return }
            let mean = self.shakes.reduce(0, +) / 10
            let sd = sqrt(self.shakes.map { ($0 - mean) * ($0 - mean) }.reduce(0, +) / 10)
            self.shakes = []
            if sd > 0.08 && self.still { self.wake() } // ~walking; a phone on a table reads ~0.01
        }
    }

    private func stopShakes() {
        if motion.isAccelerometerActive { motion.stopAccelerometerUpdates() }
    }

    /** The server's silent wake push: a Bluetooth burst. done: when iOS may suspend us again. */
    @objc public func poke(_ done: @escaping () -> Void) {
        start()
        Ble.shared.burst()
        DispatchQueue.main.asyncAfter(deadline: .now() + 20) { done() } // iOS gives a wake push ~30 s
    }

    /** Swiped away while visible: nothing restarts Kinjo, so say so. */
    @objc public func terminating() {
        guard on, Self.session != nil else { return }
        let c = UNMutableNotificationContent()
        c.title = "Kinjo is closed"
        c.body = "People near you can't see your card. Open Kinjo to stay visible."
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: "kinjo.closed", content: c, trigger: nil))
    }

    public func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard updating, let l = locations.last, l.horizontalAccuracy >= 0 else { return }
        onFix?()
        if still { // coarse fixes keep us running; only a big move counts (the accelerometer is the quick way)
            if let a = last, l.distance(from: a) > 150 + l.horizontalAccuracy { wake() }
            return
        }
        if settling {
            if l.horizontalAccuracy <= 30 { last = l; settling = false; rest() }
            return
        }
        last = l
        let now = Date()
        if let a = anchor, l.distance(from: a) <= max(Self.stillM, l.horizontalAccuracy) {
            if !foreground && now.timeIntervalSince(anchorAt) >= Self.stillAfter { settle(); return }
        } else {
            anchor = l
            anchorAt = now
        }
        if let s = sentFix, l.distance(from: s) < minMove, now.timeIntervalSince(sentAt) < next { return }
        send(l)
    }

    /** One report: over the socket, or HTTP while it's down. At most one a second. */
    private func send(_ l: CLLocation) {
        let now = Date()
        if now.timeIntervalSince(sentAt) < Self.minGap { return } // the next fix or the beat carries it
        sentAt = now
        sentFix = l
        let acc = max(1, Int(l.horizontalAccuracy.rounded()))
        let age = max(0, Int(now.timeIntervalSince(l.timestamp) * 1000))
        let r6 = { (v: Double) in (v * 1e6).rounded() / 1e6 }
        let body: [String: Any] = ["type": "pos", "lat": r6(l.coordinate.latitude), "lng": r6(l.coordinate.longitude), "acc": acc, "age": age, "still": still]
        arm()
        if let d = try? JSONSerialization.data(withJSONObject: body), let json = String(data: d, encoding: .utf8), live.send(json) { return }
        live.connect()
        Self.request("POST", "/api/presence", body) { [weak self] code, data in
            guard code == 200, let d = data, let o = (try? JSONSerialization.jsonObject(with: d)) as? [String: Any],
                  let m = o["mode"] as? String else { return } // an older server answers {ok:true}: keep the mode
            self?.server(m, o["next"] as? Int ?? 15)
        }
    }

    public func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        // no fix right now (indoors, airplane mode): Bluetooth carries on
    }

    public func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        follow()
        if auth == .notDetermined { return } // the first call comes when the manager is made, not from a choice
        let w = authWaiters
        authWaiters = []
        w.forEach { $0() }
    }

    /** iOS's "While Using" prompt; done once the person chose (at once if they already did). */
    func askWhenInUse(_ done: @escaping () -> Void) {
        guard auth == .notDetermined else { done(); return }
        authWaiters.append(done)
        lm.requestWhenInUseAuthorization()
    }

    static func openSettings() {
        if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
    }

    /** One API call with the session; done(status, body) on the main thread. 401: the session is gone, stop for good. */
    static func request(_ method: String, _ path: String, _ body: [String: Any]?, _ done: ((Int, Data?) -> Void)? = nil) {
        guard let s = session, let url = URL(string: s.api + path) else { return }
        var r = URLRequest(url: url, timeoutInterval: 10)
        r.httpMethod = method
        r.setValue("Bearer " + s.token, forHTTPHeaderField: "Authorization")
        if let body = body {
            r.setValue("application/json", forHTTPHeaderField: "Content-Type")
            r.httpBody = try? JSONSerialization.data(withJSONObject: body)
        }
        // a few seconds of runtime so a request started just before suspension still lands
        var task = UIBackgroundTaskIdentifier.invalid
        let end: () -> Void = {
            if task != .invalid { UIApplication.shared.endBackgroundTask(task); task = .invalid }
        }
        task = UIApplication.shared.beginBackgroundTask(withName: "kinjo.api", expirationHandler: end)
        URLSession.shared.dataTask(with: r) { data, resp, _ in
            DispatchQueue.main.async {
                let code = (resp as? HTTPURLResponse)?.statusCode ?? -1
                if code == 401 { Presence.shared.stop() }
                done?(code, data)
                end()
            }
        }.resume()
    }
}
