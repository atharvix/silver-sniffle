import Foundation

/**
 * The reporter socket (docs/proximity.md): fixes go up as "pos", the server answers each with
 * "mode"; "vis" false means hidden on another device. No pings either way (each would wake the
 * radio): our own reports, at least every 2 min, keep it open. Reconnects with backoff and
 * jitter; while it's down, Presence sends over HTTP. Main thread only.
 */
final class Live: NSObject, URLSessionWebSocketDelegate {
    var onMode: ((String, Int) -> Void)?
    var onHidden: (() -> Void)?
    var onOpen: (() -> Void)?
    private lazy var session = URLSession(configuration: .default, delegate: self, delegateQueue: .main)
    private var task: URLSessionWebSocketTask?
    private(set) var open = false
    private var stopped = false, retry = 0

    func connect() {
        guard !stopped, task == nil, let s = Presence.session,
              let url = URL(string: s.api.replacingOccurrences(of: "^http", with: "ws", options: .regularExpression) + "/ws") else { return }
        let t = session.webSocketTask(with: url, protocols: ["kinjo", s.token, "reporter"]) // the token rides in a header, never the URL
        task = t
        t.resume()
        receive(t)
    }

    /** False when the socket isn't open: the caller sends over HTTP instead. */
    func send(_ json: String) -> Bool {
        guard open, let t = task else { return false }
        t.send(.string(json)) { [weak self] err in
            if err != nil { DispatchQueue.main.async { self?.down(t) } }
        }
        return true
    }

    func stop() {
        stopped = true
        task?.cancel(with: .normalClosure, reason: nil)
        task = nil
        open = false
    }

    /** After stop(), for a new session (signed in again). */
    func reset() {
        stopped = false
        retry = 0
    }

    private func receive(_ t: URLSessionWebSocketTask) {
        t.receive { [weak self] result in
            DispatchQueue.main.async {
                guard let self = self, t === self.task else { return }
                switch result {
                case .success(.string(let text)):
                    self.handle(text)
                    self.receive(t)
                case .success:
                    self.receive(t)
                case .failure:
                    self.down(t)
                }
            }
        }
    }

    private func handle(_ text: String) {
        guard let d = text.data(using: .utf8), let o = (try? JSONSerialization.jsonObject(with: d)) as? [String: Any] else { return }
        switch o["type"] as? String {
        case "mode":
            onMode?(o["mode"] as? String ?? "near", o["next"] as? Int ?? 15)
        case "vis":
            if (o["visible"] as? Bool) == false { onHidden?() }
        default:
            break
        }
    }

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didOpenWithProtocol protocol: String?) {
        guard webSocketTask === task else { return }
        open = true
        retry = 0
        onOpen?() // the latest fix now: the server may have restarted
    }

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        down(webSocketTask)
    }

    private func down(_ t: URLSessionWebSocketTask) {
        guard t === task else { return }
        task = nil
        open = false
        if stopped { return }
        if (t.response as? HTTPURLResponse)?.statusCode == 401 { // the session is gone
            Presence.shared.stop()
            return
        }
        // 1 s, 2 s, 4 s … 60 s, with jitter: a server restart isn't hit by every phone at once
        let wait = min(60, pow(2, Double(min(retry, 6)))) * Double.random(in: 0.5...1.5)
        retry += 1
        DispatchQueue.main.asyncAfter(deadline: .now() + wait) { [weak self] in self?.connect() }
    }
}
