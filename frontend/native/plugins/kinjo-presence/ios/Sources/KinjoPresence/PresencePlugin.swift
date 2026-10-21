import Capacitor
import Foundation

/**
 * The same JS API as the Android plugin (window.Capacitor.Plugins.KinjoPresence), so the
 * web app needs no iPhone branch. Battery has nothing to ask on iPhone: it reports
 * "unrestricted", so its step never shows.
 */
@objc(KinjoPresencePlugin)
public class KinjoPresencePlugin: CAPPlugin, CAPBridgedPlugin {
    public let identifier = "KinjoPresencePlugin"
    public let jsName = "KinjoPresence"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "start", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "stop", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "background", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "bluetooth", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "battery", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "status", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "openSettings", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "checkPermissions", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "requestPermissions", returnType: CAPPluginReturnPromise)
    ]

    private var p: Presence { Presence.shared }

    override public func load() {
        DispatchQueue.main.async { Presence.shared.onFix = { [weak self] in self?.notifyListeners("fix", data: [:]) } }
    }

    /** Remember the session and report. Keeps going until stop() or a 401. */
    @objc func start(_ call: CAPPluginCall) {
        guard let api = call.getString("api"), let token = call.getString("token"), !token.isEmpty else {
            call.reject("api and token are required")
            return
        }
        DispatchQueue.main.async {
            self.p.save(api: api, token: token)
            self.p.start()
            call.resolve()
        }
    }

    @objc func stop(_ call: CAPPluginCall) {
        DispatchQueue.main.async { self.p.stop(); call.resolve() }
    }

    /** {granted}: "Always" location. ask:true shows iOS's choice (or Kinjo's settings once it was used). */
    @objc func background(_ call: CAPPluginCall) {
        DispatchQueue.main.async {
            if self.p.always || !call.getBool("ask", false) { call.resolve(["granted": self.p.always]); return }
            self.p.askAlways { call.resolve(["granted": $0]) }
        }
    }

    /** {granted}: Bluetooth allowed? ask:true shows iOS's prompt (only once; after that it lives in Settings). */
    @objc func bluetooth(_ call: CAPPluginCall) {
        DispatchQueue.main.async {
            let b = Ble.shared
            if b.allowed || b.asked || !call.getBool("ask", false) { call.resolve(["granted": b.allowed]); return }
            b.ask { granted in
                if granted { self.p.start() } // signed in already: start hearing now
                call.resolve(["granted": granted])
            }
        }
    }

    @objc func battery(_ call: CAPPluginCall) {
        call.resolve(["unrestricted": true, "maker": "apple"]) // iOS has no per-app battery restriction to lift
    }

    @objc func status(_ call: CAPPluginCall) {
        DispatchQueue.main.async {
            call.resolve(["located": self.p.located, "background": self.p.always, "bluetoothAllowed": Ble.shared.allowed, "bleHeard": Ble.shared.heard])
        }
    }

    @objc func openSettings(_ call: CAPPluginCall) {
        DispatchQueue.main.async { Presence.openSettings(); call.resolve() }
    }

    @objc override public func checkPermissions(_ call: CAPPluginCall) {
        DispatchQueue.main.async { call.resolve(self.state()) }
    }

    /** requestPermissions({permissions:["location"]}): iOS's "while using" prompt. */
    @objc override public func requestPermissions(_ call: CAPPluginCall) {
        DispatchQueue.main.async { self.p.askWhenInUse { call.resolve(self.state()) } }
    }

    private func state() -> [String: Any] {
        ["location": p.located ? "granted" : p.auth == .notDetermined ? "prompt" : "denied"]
    }
}
