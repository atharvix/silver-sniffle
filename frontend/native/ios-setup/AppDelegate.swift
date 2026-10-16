import Capacitor
import FirebaseCore
import FirebaseMessaging
import KinjoPresence
import UIKit

// Replaces the generated AppDelegate (ios-build.sh copies it in). Capacitor's standard
// hooks, plus: FCM tokens instead of raw APNs ones (the server sends every push through
// FCM, which rejects APNs tokens), and the server's silent "wake" push.
@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        FirebaseApp.configure()
        Presence.shared.resumeIfSaved() // relaunched in the background (location event, wake push): keep reporting
        return true
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        Messaging.messaging().apnsToken = deviceToken
        Messaging.messaging().token { token, error in
            if let error = error {
                NotificationCenter.default.post(name: .capacitorDidFailToRegisterForRemoteNotifications, object: error)
            } else if let token = token {
                NotificationCenter.default.post(name: .capacitorDidRegisterForRemoteNotifications, object: token)
            }
        }
    }

    func application(_ application: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        NotificationCenter.default.post(name: .capacitorDidFailToRegisterForRemoteNotifications, object: error)
    }

    func application(_ application: UIApplication, didReceiveRemoteNotification userInfo: [AnyHashable: Any], fetchCompletionHandler completionHandler: @escaping (UIBackgroundFetchResult) -> Void) {
        guard userInfo["kind"] as? String == "wake" else { completionHandler(.noData); return }
        Presence.shared.poke { completionHandler(.newData) }
    }

    func applicationWillTerminate(_ application: UIApplication) {
        Presence.shared.terminating() // swiped away while visible: nothing restarts Kinjo, so ask to reopen it
    }

    func application(_ app: UIApplication, open url: URL, options: [UIApplication.OpenURLOptionsKey: Any] = [:]) -> Bool {
        return ApplicationDelegateProxy.shared.application(app, open: url, options: options) // kinjo://auth
    }

    func application(_ application: UIApplication, continue userActivity: NSUserActivity, restorationHandler: @escaping ([UIUserActivityRestoring]?) -> Void) -> Bool {
        return ApplicationDelegateProxy.shared.application(application, continue: userActivity, restorationHandler: restorationHandler)
    }
}
