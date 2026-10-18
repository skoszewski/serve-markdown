using System.ComponentModel;
using System.Reflection;
using Microsoft.Win32;

namespace ServeMarkdown;

static class Program
{
    /// <summary>Runs the notification-area app serving folders and Markdown files with serve-markdown.</summary>
    [STAThread]
    static void Main()
    {
        ApplicationConfiguration.Initialize();
        Application.Run(new TrayContext());
    }
}

/// <summary>
/// The notification-area icon and its menu; stops every instance before the app exits.
/// </summary>
sealed class TrayContext : ApplicationContext
{
    const string RunKey = @"Software\Microsoft\Windows\CurrentVersion\Run";
    const string RunValue = "ServeMarkdown";

    readonly ContextMenuStrip menu = new();
    readonly Settings settings = Settings.Load();
    readonly NotifyIcon icon;
    readonly InstanceManager manager;

    public TrayContext()
    {
        manager = new InstanceManager(SynchronizationContext.Current!, settings);
        icon = new NotifyIcon
        {
            Icon = SystemIcons.GetStockIcon(StockIconId.DocumentWithAssociation, StockIconOptions.SmallIcon),
            Text = "Serve Markdown",
            ContextMenuStrip = menu,
            Visible = true,
        };
        menu.Opening += OnMenuOpening;
        SystemEvents.SessionEnding += (_, _) => manager.StopAll(() => { });
    }

    protected override void ExitThreadCore()
    {
        icon.Visible = false;
        icon.Dispose();
        base.ExitThreadCore();
    }

    /// <summary>Builds the menu anew from the instances and the settings each time it opens.</summary>
    void OnMenuOpening(object? sender, CancelEventArgs e)
    {
        menu.Items.Clear();

        menu.Items.Add("Serve Folder...", null, (_, _) =>
        {
            using var dialog = new FolderBrowserDialog { Description = "Serve Folder", UseDescriptionForTitle = true };
            if (dialog.ShowDialog() == DialogResult.OK)
            {
                manager.Start(dialog.SelectedPath, settings.Editor);
            }
        });
        menu.Items.Add("Serve File...", null, (_, _) =>
        {
            using var dialog = new OpenFileDialog
            {
                Title = "Serve File",
                Filter = "Markdown files (*.md;*.markdown)|*.md;*.markdown",
            };
            if (dialog.ShowDialog() == DialogResult.OK)
            {
                manager.Start(dialog.FileName, settings.Editor);
            }
        });
        if (settings.Recent.Count > 0)
        {
            var recent = new ToolStripMenuItem("Serve Recent");
            foreach (var path in settings.Recent)
            {
                recent.DropDownItems.Add(path, null, (_, _) => manager.Start(path, settings.Editor));
            }
            recent.DropDownItems.Add(new ToolStripSeparator());
            recent.DropDownItems.Add("Clear Menu", null, (_, _) =>
            {
                settings.Recent.Clear();
                settings.Save();
            });
            menu.Items.Add(recent);
        }
        menu.Items.Add(new ToolStripMenuItem("Enable Editor", null, (_, _) =>
        {
            settings.Editor = !settings.Editor;
            settings.Save();
        }) { Checked = settings.Editor });

        if (manager.Instances.Count > 0)
        {
            menu.Items.Add(new ToolStripSeparator());
            foreach (var instance in manager.Instances)
            {
                var item = new ToolStripMenuItem($"{instance.Name} - :{instance.Port}");
                item.DropDownItems.Add("Open in Browser", null, (_, _) => InstanceManager.Open(instance.Url));
                if (instance.Editor)
                {
                    item.DropDownItems.Add("Open Editor", null, (_, _) => InstanceManager.Open(instance.EditorUrl));
                }
                item.DropDownItems.Add("Show Log", null, (_, _) => InstanceManager.Open(instance.LogPath));
                item.DropDownItems.Add("Stop", null, (_, _) => manager.Stop(instance));
                menu.Items.Add(item);
            }
            menu.Items.Add("Stop All", null, (_, _) => manager.StopAll(() => { }));
        }

        // Launch at Login is the Run value of the user's registry naming this executable.
        menu.Items.Add(new ToolStripSeparator());
        var command = $"\"{Environment.ProcessPath}\"";
        using (var key = Registry.CurrentUser.CreateSubKey(RunKey))
        {
            var enabled = key.GetValue(RunValue) as string == command;
            menu.Items.Add(new ToolStripMenuItem("Launch at Login", null, (_, _) =>
            {
                using var runKey = Registry.CurrentUser.CreateSubKey(RunKey);
                if (enabled)
                {
                    runKey.DeleteValue(RunValue, throwOnMissingValue: false);
                }
                else
                {
                    runKey.SetValue(RunValue, command);
                }
            }) { Checked = enabled });
        }

        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("About Serve Markdown", null, (_, _) =>
        {
            var copyright = Assembly.GetExecutingAssembly().GetCustomAttribute<AssemblyCopyrightAttribute>()?.Copyright;
            MessageBox.Show($"Serve Markdown {Application.ProductVersion}\n\n{copyright}", "About Serve Markdown",
                MessageBoxButtons.OK, MessageBoxIcon.Information);
        });
        menu.Items.Add("Quit Serve Markdown", null, (_, _) => manager.StopAll(ExitThread));

        e.Cancel = false;
    }
}
