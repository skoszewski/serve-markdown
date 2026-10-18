using System.Diagnostics;
using System.Net;
using System.Net.Sockets;
using System.Text.Json;

namespace ServeMarkdown;

/// <summary>
/// The settings of the app, kept in %LOCALAPPDATA%\ServeMarkdown\settings.json.
/// </summary>
sealed class Settings
{
    static readonly string FilePath = Path.Combine(InstanceManager.DataDirectory, "settings.json");

    /// <summary>Whether the next instance runs the editor.</summary>
    public bool Editor { get; set; }

    /// <summary>The folders and files served last, the latest first.</summary>
    public List<string> Recent { get; set; } = [];

    /// <summary>Reads the settings, or returns the defaults when the file is missing or unreadable.</summary>
    public static Settings Load()
    {
        try
        {
            return JsonSerializer.Deserialize<Settings>(File.ReadAllText(FilePath)) ?? new();
        }
        catch (Exception e) when (e is IOException or UnauthorizedAccessException or JsonException)
        {
            return new();
        }
    }

    /// <summary>Writes the settings.</summary>
    public void Save()
    {
        Directory.CreateDirectory(InstanceManager.DataDirectory);
        File.WriteAllText(FilePath, JsonSerializer.Serialize(this));
    }
}

/// <summary>
/// A serve-markdown process the app started, serving one folder or Markdown file.
/// </summary>
sealed class Instance
{
    public required string Name { get; init; }
    public required int Port { get; init; }
    public required bool Editor { get; init; }
    public required string LogPath { get; init; }
    public required StreamWriter Log { get; init; }
    public required Process Process { get; init; }
    public bool Stopping { get; set; }

    public string Url => $"http://127.0.0.1:{Port}/";
    public string EditorUrl => $"http://127.0.0.1:{Port}/_/editor/";
}

/// <summary>
/// Starts, lists and stops the serve-markdown processes of the app, on the thread of context.
/// </summary>
sealed class InstanceManager(SynchronizationContext context, Settings settings)
{
    public static readonly string DataDirectory = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "ServeMarkdown");

    readonly List<Instance> instances = [];
    Action? allStopped;

    public IReadOnlyList<Instance> Instances => instances;

    /// <summary>
    /// Serves the folder or file at path on a free port of 127.0.0.1, with or without the
    /// editor, logging to %LOCALAPPDATA%\ServeMarkdown\Logs.
    /// </summary>
    public void Start(string path, bool editor)
    {
        var name = Path.GetFileName(Path.TrimEndingDirectorySeparator(path));

        // A free port is asked of the system and released for serve-markdown to bind.
        var listener = new TcpListener(IPAddress.Loopback, 0);
        listener.Start();
        var port = ((IPEndPoint)listener.LocalEndpoint).Port;
        listener.Stop();

        // The log of the instance is written anew for each start.
        var logs = Path.Combine(DataDirectory, "Logs");
        var logPath = Path.Combine(logs, $"{name}-{port}.log");
        StreamWriter log;
        try
        {
            Directory.CreateDirectory(logs);
            log = new StreamWriter(logPath, append: false) { AutoFlush = true };
        }
        catch (Exception e) when (e is IOException or UnauthorizedAccessException)
        {
            ShowWarning($"Cannot serve {name}", e.Message);
            return;
        }

        var info = new ProcessStartInfo(Path.Combine(AppContext.BaseDirectory, "serve-markdown.exe"))
        {
            ArgumentList =
            {
                "--listen-address", "127.0.0.1", "--port", port.ToString(),
                $"--editor={(editor ? "true" : "false")}", "--stop-on-stdin-eof", path,
            },
            WorkingDirectory = Directory.Exists(path) ? path : Path.GetDirectoryName(path)!,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };
        var process = new Process { StartInfo = info, EnableRaisingEvents = true };
        var instance = new Instance
        {
            Name = name, Port = port, Editor = editor, LogPath = logPath, Log = log, Process = process,
        };

        DataReceivedEventHandler write = (_, e) =>
        {
            if (e.Data is not null)
            {
                lock (log)
                {
                    log.WriteLine(e.Data);
                }
            }
        };
        process.OutputDataReceived += write;
        process.ErrorDataReceived += write;
        process.Exited += (_, _) =>
        {
            process.WaitForExit();
            context.Post(_ => Finished(instance), null);
        };

        try
        {
            process.Start();
        }
        catch (Exception e) when (e is System.ComponentModel.Win32Exception or InvalidOperationException)
        {
            log.Dispose();
            ShowWarning($"Cannot serve {name}", e.Message);
            return;
        }
        process.BeginOutputReadLine();
        process.BeginErrorReadLine();
        instances.Add(instance);

        settings.Recent.RemoveAll(recent => string.Equals(recent, path, StringComparison.OrdinalIgnoreCase));
        settings.Recent.Insert(0, path);
        if (settings.Recent.Count > 5)
        {
            settings.Recent.RemoveRange(5, settings.Recent.Count - 5);
        }
        settings.Save();
    }

    /// <summary>
    /// Closes the standard input of the instance, and kills it if it still runs 6 seconds later.
    /// </summary>
    public async void Stop(Instance instance)
    {
        instance.Stopping = true;
        instance.Process.StandardInput.Close();
        await Task.Delay(TimeSpan.FromSeconds(6));
        if (instances.Contains(instance))
        {
            Kill(instance);
        }
    }

    /// <summary>
    /// Stops every instance and calls done once the last has exited, or after 6 seconds,
    /// killing those still running then.
    /// </summary>
    public async void StopAll(Action done)
    {
        if (instances.Count == 0)
        {
            done();
            return;
        }
        allStopped = done;
        foreach (var instance in instances)
        {
            instance.Stopping = true;
            instance.Process.StandardInput.Close();
        }
        await Task.Delay(TimeSpan.FromSeconds(6));
        if (allStopped is not { } pending)
        {
            return;
        }
        allStopped = null;
        foreach (var instance in instances)
        {
            Kill(instance);
        }
        pending();
    }

    /// <summary>Opens a URL or a file with the program Windows associates with it.</summary>
    public static void Open(string target)
    {
        Process.Start(new ProcessStartInfo(target) { UseShellExecute = true });
    }

    static void Kill(Instance instance)
    {
        try
        {
            instance.Process.Kill();
        }
        catch (InvalidOperationException)
        {
        }
    }

    /// <summary>Removes the instance that exited, and reports an exit nobody asked for.</summary>
    void Finished(Instance instance)
    {
        if (!instances.Remove(instance))
        {
            return;
        }
        lock (instance.Log)
        {
            instance.Log.Dispose();
        }
        var exitCode = instance.Process.ExitCode;
        instance.Process.Dispose();

        if (instances.Count == 0 && allStopped is { } done)
        {
            allStopped = null;
            done();
        }
        if (instance.Stopping)
        {
            return;
        }

        string lines;
        try
        {
            lines = string.Join("\n", File.ReadLines(instance.LogPath).TakeLast(12)).Trim();
        }
        catch (Exception e) when (e is IOException or UnauthorizedAccessException)
        {
            lines = "";
        }
        var showLog = new TaskDialogButton("Show Log");
        var page = new TaskDialogPage
        {
            Caption = "Serve Markdown",
            Heading = $"{instance.Name} stopped",
            Text = $"serve-markdown exited with code {exitCode}.\n\n{lines}",
            Icon = TaskDialogIcon.Warning,
            Buttons = { TaskDialogButton.OK, showLog },
        };
        if (TaskDialog.ShowDialog(page) == showLog)
        {
            Open(instance.LogPath);
        }
    }

    static void ShowWarning(string heading, string text)
    {
        MessageBox.Show(text, heading, MessageBoxButtons.OK, MessageBoxIcon.Warning);
    }
}
