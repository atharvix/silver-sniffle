import CoreLocation
import Foundation

/**
 * Location Push Service Extension: iOS starts this (not the app) when the server sends an
 * APNs `location` push to the token Presence registered. It runs even when Kinjo isn't
 * running. One fix goes to POST /api/presence, the same as the app's, which keeps the card
 * on decks for the server's 2 minutes. Session from the app group Presence writes; none
 * (hidden, signed out) means do nothing.
 */
class LocationPushService: NSObject, CLLocationPushServiceExtension, CLLocationManagerDelegate {
    static let group = "group.com.kinjo.app", waitFor: TimeInterval = 15, goodAcc: CLLocationAccuracy = 20

    private var lm: CLLocationManager?
    private var done: (() -> Void)?
    private var best: CLLocation?
    private var startedAt = Date()
    private var session: AnyObject? // CLServiceSession(.always), iOS 18+: Apple asks the extension to hold one too

    func didReceiveLocationPushPayload(_ payload: [String: Any], completion: @escaping () -> Void) {
        DispatchQueue.main.async {
            let prefs = UserDefaults(suiteName: Self.group)
            guard prefs?.string(forKey: "kinjo.presence.token") != nil else { completion(); return }
            if #available(iOS 18.0, *) { self.session = CLServiceSession(authorization: .always) }
            self.done = completion
            self.startedAt = Date()
            let m = CLLocationManager()
            m.delegate = self
            m.desiredAccuracy = kCLLocationAccuracyBest
            m.distanceFilter = kCLDistanceFilterNone
            self.lm = m
            m.startUpdatingLocation()
            DispatchQueue.main.asyncAfter(deadline: .now() + Self.waitFor) { self.finish(self.best) }
        }
    }

    /** iOS is about to end the extension: send what we have. */
    func serviceExtensionWillTerminate() {
        DispatchQueue.main.async { self.finish(self.best, wait: false) }
    }

    func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        for l in locations where l.horizontalAccuracy >= 0 && l.timestamp >= startedAt.addingTimeInterval(-10) {
            if best.map({ l.horizontalAccuracy <= $0.horizontalAccuracy }) ?? true { best = l }
        }
        if let b = best, b.horizontalAccuracy <= Self.goodAcc { finish(b) }
    }

    func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        // keep waiting until waitFor: a fix may still come
    }

    private func finish(_ l: CLLocation?, wait: Bool = true) {
        guard let complete = done else { return }
        done = nil
        lm?.stopUpdatingLocation()
        lm = nil
        guard let l = l, let r = request(l) else { complete(); return }
        if !wait { URLSession.shared.dataTask(with: r).resume(); complete(); return }
        URLSession.shared.dataTask(with: r) { _, _, _ in complete() }.resume()
    }

    private func request(_ l: CLLocation) -> URLRequest? {
        let prefs = UserDefaults(suiteName: Self.group)
        guard let api = prefs?.string(forKey: "kinjo.presence.api"), let token = prefs?.string(forKey: "kinjo.presence.token"),
              let url = URL(string: api + "/api/presence") else { return nil }
        let acc = max(1, Int(l.horizontalAccuracy.rounded()))
        let age = max(0, Int(Date().timeIntervalSince(l.timestamp) * 1000))
        let r6 = { (v: Double) in (v * 1e6).rounded() / 1e6 }
        var r = URLRequest(url: url, timeoutInterval: 8)
        r.httpMethod = "POST"
        r.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
        r.setValue("application/json", forHTTPHeaderField: "Content-Type")
        r.httpBody = try? JSONSerialization.data(withJSONObject: ["lat": r6(l.coordinate.latitude), "lng": r6(l.coordinate.longitude), "acc": acc, "age": age])
        return r
    }
}
