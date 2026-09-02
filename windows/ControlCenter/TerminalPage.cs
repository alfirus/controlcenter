namespace ControlCenter;
using Microsoft.UI.Xaml.Controls;
// Minimal WinUI 3 terminal page — ConPTY + Windows Terminal control would bind here.
// WS: ws://localhost:8080/v1/terminal/sessions/{id}/ws (binary PTY + JSON resize)
public sealed class TerminalPage : Page {
    public TerminalPage() {
        Content = new TextBlock { Text = "Terminal — ConPTY B&W\nWS: ws://localhost:8080/v1/terminal/sessions/:id/ws", FontFamily = new Microsoft.UI.Xaml.Media.FontFamily("Cascadia Mono") };
    }
}
