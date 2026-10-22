import AuthenticationServices
import CryptoKit
import Foundation
import UIKit

/**
 * iPhone sign-in sheets:
 * - Sign in with Apple (Apple's own sheet, Face ID): hands back the signed identity token
 *   for the backend to verify. Only the email is asked for: people type their own name.
 * - Google/LinkedIn in ASWebAuthenticationSession instead of an in-app Safari view: it shares
 *   Safari's cookies, so an account already signed in to Google in Safari is offered, not typed.
 * Each one keeps itself alive until it's done.
 */
final class SignIn: NSObject, ASAuthorizationControllerDelegate, ASAuthorizationControllerPresentationContextProviding,
    ASWebAuthenticationPresentationContextProviding {
    private static var live = Set<SignIn>()
    private let anchor: ASPresentationAnchor
    private var done: ((Result<[String: Any], Error>) -> Void)?
    private var session: ASWebAuthenticationSession?

    private init(anchor: ASPresentationAnchor) { self.anchor = anchor }

    /** nonce: the app's random string; Apple signs its SHA-256 into the token. */
    static func apple(anchor: ASPresentationAnchor, nonce: String, done: @escaping (Result<[String: Any], Error>) -> Void) {
        let s = SignIn(anchor: anchor)
        s.done = done
        live.insert(s)
        let req = ASAuthorizationAppleIDProvider().createRequest()
        req.requestedScopes = [.email]
        req.nonce = SHA256.hash(data: Data(nonce.utf8)).map { String(format: "%02x", $0) }.joined()
        let c = ASAuthorizationController(authorizationRequests: [req])
        c.delegate = s
        c.presentationContextProvider = s
        c.performRequests()
    }

    /** Opens url; resolves with the kinjo:// URL the backend redirects to at the end. */
    static func web(anchor: ASPresentationAnchor, url: URL, scheme: String, done: @escaping (Result<[String: Any], Error>) -> Void) {
        let s = SignIn(anchor: anchor)
        s.done = done
        live.insert(s)
        let session = ASWebAuthenticationSession(url: url, callbackURLScheme: scheme) { back, err in
            if let back = back { s.finish(.success(["url": back.absoluteString])) }
            else { s.finish(.failure(err ?? NSError(domain: "kinjo", code: 1))) }
        }
        session.presentationContextProvider = s
        session.prefersEphemeralWebBrowserSession = false // share Safari's sign-ins
        s.session = session
        if !session.start() { s.finish(.failure(NSError(domain: "kinjo", code: 2))) }
    }

    private func finish(_ r: Result<[String: Any], Error>) {
        done?(r)
        done = nil
        session = nil
        SignIn.live.remove(self)
    }

    func presentationAnchor(for controller: ASAuthorizationController) -> ASPresentationAnchor { anchor }
    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor { anchor }

    func authorizationController(controller: ASAuthorizationController, didCompleteWithAuthorization auth: ASAuthorization) {
        guard let cred = auth.credential as? ASAuthorizationAppleIDCredential,
              let tok = cred.identityToken, let idToken = String(data: tok, encoding: .utf8) else {
            finish(.failure(NSError(domain: "kinjo", code: 3)))
            return
        }
        finish(.success(["idToken": idToken]))
    }

    func authorizationController(controller: ASAuthorizationController, didCompleteWithError error: Error) {
        finish(.failure(error))
    }
}
