import AppKit

/// Brings the app to the front, so the panel or alert it opens next takes the focus.
@MainActor
func activateApp() {
    if #available(macOS 14, *) {
        NSApp.activate()
    } else {
        NSApp.activate(ignoringOtherApps: true)
    }
}

/// A serve-markdown process the app started, serving one folder or Markdown file.
@MainActor
final class Instance: Identifiable {
    let id = UUID()
    let name: String
    let port: Int
    let editor: Bool
    let logURL: URL
    let log: FileHandle
    let process: Process
    var stopping = false

    init(name: String, port: Int, editor: Bool, logURL: URL, log: FileHandle, process: Process) {
        self.name = name
        self.port = port
        self.editor = editor
        self.logURL = logURL
        self.log = log
        self.process = process
    }

    var url: URL { URL(string: "http://127.0.0.1:\(port)/")! }
    var editorURL: URL { URL(string: "http://127.0.0.1:\(port)/_/editor/")! }
}

/// Starts, lists and stops the serve-markdown processes of the app.
@MainActor
final class InstanceManager: ObservableObject {
    @Published private(set) var instances: [Instance] = []
    private var allStopped: (() -> Void)?

    /// Serves the folder or file at url on a free port of 127.0.0.1, with or without the
    /// editor, logging to ~/Library/Logs/ServeMarkdown.
    func start(_ url: URL, editor: Bool) {
        let name = url.lastPathComponent

        // A free port is asked of the system and released for serve-markdown to bind.
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        let bound = fd >= 0 && withUnsafeMutablePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                bind(fd, $0, length) == 0 && getsockname(fd, $0, &length) == 0
            }
        }
        if fd >= 0 {
            close(fd)
        }
        guard bound else {
            showAlert("Cannot serve \(name)", "No free port was found: \(String(cString: strerror(errno))).")
            return
        }
        let port = Int(UInt16(bigEndian: address.sin_port))

        // The log of the instance is written anew for each start.
        let logs = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Logs/ServeMarkdown", isDirectory: true)
        let logURL = logs.appendingPathComponent("\(name)-\(port).log")
        let log: FileHandle
        do {
            try FileManager.default.createDirectory(at: logs, withIntermediateDirectories: true)
            try Data().write(to: logURL)
            log = try FileHandle(forWritingTo: logURL)
        } catch {
            showAlert("Cannot serve \(name)", error.localizedDescription)
            return
        }

        guard let executable = Bundle.main.url(forAuxiliaryExecutable: "serve-markdown") else {
            try? log.close()
            showAlert("Cannot serve \(name)", "serve-markdown is missing from the app bundle.")
            return
        }
        let isDirectory = (try? url.resourceValues(forKeys: [.isDirectoryKey]).isDirectory) ?? false
        let process = Process()
        process.executableURL = executable
        process.arguments = ["--listen-address", "127.0.0.1", "--port", String(port), "--editor=\(editor)", url.path]
        process.currentDirectoryURL = isDirectory ? url : url.deletingLastPathComponent()
        process.standardInput = FileHandle.nullDevice
        process.standardOutput = log
        process.standardError = log

        let instance = Instance(name: name, port: port, editor: editor, logURL: logURL, log: log, process: process)
        let id = instance.id
        process.terminationHandler = { [weak self] _ in
            Task { @MainActor in
                self?.finished(id)
            }
        }
        do {
            try process.run()
        } catch {
            try? log.close()
            showAlert("Cannot serve \(name)", error.localizedDescription)
            return
        }
        instances.append(instance)
    }

    /// Interrupts the instance, and terminates it if it still runs 6 seconds later.
    func stop(_ instance: Instance) {
        instance.stopping = true
        instance.process.interrupt()
        Task {
            try? await Task.sleep(for: .seconds(6))
            if instance.process.isRunning {
                instance.process.terminate()
            }
        }
    }

    /// Stops every instance and calls done once the last has exited, or after 6 seconds,
    /// terminating those still running then.
    func stopAll(then done: @escaping () -> Void) {
        if instances.isEmpty {
            done()
            return
        }
        allStopped = done
        for instance in instances {
            instance.stopping = true
            instance.process.interrupt()
        }
        Task {
            try? await Task.sleep(for: .seconds(6))
            guard let done = allStopped else {
                return
            }
            allStopped = nil
            for instance in instances where instance.process.isRunning {
                instance.process.terminate()
            }
            done()
        }
    }

    /// Removes the instance that exited, and reports an exit nobody asked for.
    private func finished(_ id: UUID) {
        guard let index = instances.firstIndex(where: { $0.id == id }) else {
            return
        }
        let instance = instances.remove(at: index)
        try? instance.log.close()

        if instances.isEmpty, let done = allStopped {
            allStopped = nil
            done()
        }
        if instance.stopping {
            return
        }

        let process = instance.process
        let status = process.terminationReason == .uncaughtSignal
            ? "was killed by signal \(process.terminationStatus)"
            : "exited with status \(process.terminationStatus)"
        let text = (try? String(contentsOf: instance.logURL, encoding: .utf8)) ?? ""
        let lines = text.split(separator: "\n", omittingEmptySubsequences: false).suffix(12).joined(separator: "\n")
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = "\(instance.name) stopped"
        alert.informativeText = "serve-markdown \(status).\n\n\(lines.trimmingCharacters(in: .whitespacesAndNewlines))"
        alert.addButton(withTitle: "OK")
        alert.addButton(withTitle: "Show Log")
        activateApp()
        if alert.runModal() == .alertSecondButtonReturn {
            NSWorkspace.shared.open(instance.logURL)
        }
    }

    private func showAlert(_ message: String, _ information: String) {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = message
        alert.informativeText = information
        activateApp()
        alert.runModal()
    }
}
