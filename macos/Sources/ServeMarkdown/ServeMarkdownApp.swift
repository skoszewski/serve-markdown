import AppKit
import ServiceManagement
import SwiftUI
import UniformTypeIdentifiers

/// The menu bar app serving folders and Markdown files with serve-markdown.
@main
struct ServeMarkdownApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate

    var body: some Scene {
        MenuBarExtra("Serve Markdown", systemImage: "doc.richtext") {
            MenuContent(manager: appDelegate.manager)
        }
        .menuBarExtraStyle(.menu)
    }
}

/// Owns the instances, and stops them all before the app quits.
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let manager = InstanceManager()

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if manager.instances.isEmpty {
            return .terminateNow
        }
        manager.stopAll {
            NSApp.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }
}

/// The items of the menu bar menu.
struct MenuContent: View {
    @ObservedObject var manager: InstanceManager
    @AppStorage("editor") private var editor = false

    var body: some View {
        Button("Serve Folder or File...") {
            // The panel opens once the menu has closed.
            DispatchQueue.main.async { [manager, editor] in
                activateApp()
                let panel = NSOpenPanel()
                panel.canChooseDirectories = true
                panel.canChooseFiles = true
                panel.allowsMultipleSelection = false
                panel.allowedContentTypes = [.folder] + ["md", "markdown"].compactMap { UTType(filenameExtension: $0) }
                panel.prompt = "Serve"
                if panel.runModal() == .OK, let url = panel.url {
                    manager.start(url, editor: editor)
                }
            }
        }
        if !manager.recent.isEmpty {
            Menu("Serve Recent") {
                ForEach(manager.recent, id: \.path) { url in
                    Button((url.path as NSString).abbreviatingWithTildeInPath) {
                        manager.start(url, editor: editor)
                    }
                }
                Divider()
                Button("Clear Menu") {
                    manager.recent.removeAll()
                }
            }
        }
        Toggle("Enable Editor", isOn: $editor)

        if !manager.instances.isEmpty {
            Divider()
            ForEach(manager.instances) { instance in
                Menu("\(instance.name) - :\(String(instance.port))") {
                    Button("Open in Browser") {
                        NSWorkspace.shared.open(instance.url)
                    }
                    if instance.editor {
                        Button("Open Editor") {
                            NSWorkspace.shared.open(instance.editorURL)
                        }
                    }
                    Button("Show Log") {
                        NSWorkspace.shared.open(instance.logURL)
                    }
                    Button("Stop") {
                        manager.stop(instance)
                    }
                }
            }
            Button("Stop All") {
                manager.stopAll {}
            }
        }

        Divider()
        Toggle("Launch at Login", isOn: Binding(
            get: { SMAppService.mainApp.status == .enabled },
            set: { enabled in
                do {
                    if enabled {
                        try SMAppService.mainApp.register()
                    } else {
                        try SMAppService.mainApp.unregister()
                    }
                } catch {
                    activateApp()
                    NSAlert(error: error).runModal()
                }
                if SMAppService.mainApp.status == .requiresApproval {
                    SMAppService.openSystemSettingsLoginItems()
                }
                // The menu reads the status again when it is drawn anew.
                manager.objectWillChange.send()
            }
        ))
        Divider()
        Button("About Serve Markdown") {
            // The panel opens once the menu has closed.
            DispatchQueue.main.async {
                activateApp()
                NSApp.orderFrontStandardAboutPanel(nil)
            }
        }
        Button("Quit Serve Markdown") {
            NSApp.terminate(nil)
        }
        .keyboardShortcut("q")
    }
}
