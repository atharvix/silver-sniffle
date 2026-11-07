import CoreLocation
import Foundation
import UIKit
import UserNotifications

/**
 * The signed-in session and the optional location fix (docs/proximity.md). Bluetooth (Ble)
 * is what finds people; location is used only while the app is open ("While Using"): one fix
 * every fgEvery to POST /api/presence, an outdoor check for the server. No background location.
 * Main thread only.
 */
@objc public final class Presence: NSObject, CLLocationManagerDelegate {
    @objc public static let shared = Presence()

    static let fgEvery: TimeInterval = 10
    static let group = "group.com.kinjo.app"
    private static let apiKey = "kinjo.presence.api", tokenKey = "kinjo.presence.token"

    /** The session (the app group's defaults when the entitlement is there, else the app's own). */
    static let store = UserDefaults(suiteName: Presence.group) ?? .standard
    private var prefs: UserDefaults { Self.store }
    private let lm = CLLocationManager()
    private var on = false, foreground = true, updating = false
    private var sentAt = Date.distantPast
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
        lm.distanceFilter = kCLDistanceFilterNone
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
        foreground = UIApplication.shared.applicationState != .background
        Ble.shared.start() // no-op until Bluetooth is allowed
        follow()
    }

    func stop() {
        on = false
        Ble.shared.stop()
        follow()
        save(api: nil, token: nil)
    }

    /** Location updates only while signed in, allowed and on screen. */
    private func follow() {
        let want = on && located && foreground
        if want == updating { return }
        updating = want
        if want { sentAt = .distantPast; lm.startUpdatingLocation() } else { lm.stopUpdatingLocation() }
    }

    @objc private func toForeground() { foreground = true; follow() }
    @objc private func toBackground() { foreground = false; follow() }

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
        guard let l = locations.last else { return }
        onFix?()
        guard updating, Date().timeIntervalSince(sentAt) >= Self.fgEvery else { return }
        sentAt = Date()
        let acc = l.horizontalAccuracy >= 0 ? max(1, Int(l.horizontalAccuracy.rounded())) : 0 // 0 = unknown: the server won't match on it
        let age = max(0, Int(Date().timeIntervalSince(l.timestamp) * 1000))
        let r6 = { (v: Double) in (v * 1e6).rounded() / 1e6 }
        Self.request("POST", "/api/presence", ["lat": r6(l.coordinate.latitude), "lng": r6(l.coordinate.longitude), "acc": acc, "age": age])
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
