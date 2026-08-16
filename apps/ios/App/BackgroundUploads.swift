import Foundation

extension Notification.Name {
    static let backgroundUploadFinished = Notification.Name("EchoTrails.backgroundUploadFinished")
}

final class BackgroundUploadCoordinator: NSObject, URLSessionTaskDelegate, URLSessionDelegate {
    static let shared = BackgroundUploadCoordinator()
    private var completionHandler: (() -> Void)?
    private let schedulingLock = NSLock()
    private var scheduledUploadIDs: Set<String> = []
    private lazy var session: URLSession = {
        let configuration = URLSessionConfiguration.background(withIdentifier: "tv.subcult.echotrails.uploads")
        configuration.sessionSendsLaunchEvents = true
        configuration.isDiscretionary = false
        configuration.waitsForConnectivity = true
        configuration.allowsCellularAccess = true
        return URLSession(configuration: configuration, delegate: self, delegateQueue: nil)
    }()

    func upload(fileURL: URL, to signedURL: URL, contentType: String, uploadID: String) {
        // Restored background tasks survive process death. Inspect them before
        // scheduling so replaying a composer checkpoint cannot create a second
        // PUT for the same upload record.
        session.getAllTasks { [weak self] tasks in
            guard let self else { return }
            schedulingLock.lock()
            defer { schedulingLock.unlock() }
            guard !scheduledUploadIDs.contains(uploadID),
                  !tasks.contains(where: { $0.taskDescription == uploadID && $0.state != .completed }) else { return }
            scheduledUploadIDs.insert(uploadID)
            var request = URLRequest(url: signedURL); request.httpMethod = "PUT"; request.setValue(contentType, forHTTPHeaderField: "Content-Type")
            let task = session.uploadTask(with: request, fromFile: fileURL); task.taskDescription = uploadID; task.resume()
        }
    }

    func restoreTasks() async -> [URLSessionTask] { await session.allTasks }
    func setBackgroundCompletion(_ completion: @escaping () -> Void) { completionHandler = completion }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        schedulingLock.lock(); scheduledUploadIDs.remove(task.taskDescription ?? ""); schedulingLock.unlock()
        let statusCode = (task.response as? HTTPURLResponse)?.statusCode ?? 0
        let succeeded = error == nil && (200..<300).contains(statusCode)
        NotificationCenter.default.post(name: .backgroundUploadFinished, object: nil, userInfo: ["upload_id": task.taskDescription ?? "", "success": succeeded])
    }

    func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        DispatchQueue.main.async { self.completionHandler?(); self.completionHandler = nil }
    }
}
