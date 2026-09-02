#include <gtk/gtk.h>
#include <vte/vte.h>
// Minimal Ubuntu terminal — libvte widget wiring to WS /terminal/sessions/:id/ws
// Build: pkg-config --cflags --libs vte-2.91 gtk4
// For now, placeholder that shows B&W host list; real PTY connects via libsoup websocket.

static void activate(GtkApplication *app, gpointer data) {
    (void)data;
    GtkWidget *win = gtk_application_window_new(app);
    gtk_window_set_title(GTK_WINDOW(win), "Control Center — Terminal (libvte B&W)");
    gtk_window_set_default_size(GTK_WINDOW(win), 800, 600);
    // Placeholder label; real: VteTerminal *term = vte_terminal_new();
    GtkWidget *label = gtk_label_new("Terminal — libvte B&W\nWS: ws://localhost:8080/v1/terminal/sessions/:id/ws");
    gtk_window_set_child(GTK_WINDOW(win), label);
    gtk_window_present(GTK_WINDOW(win));
}
int main(int argc, char **argv) {
    GtkApplication *app = gtk_application_new("com.alfirus.controlcenter", G_APPLICATION_DEFAULT_FLAGS);
    g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
    int s = g_application_run(G_APPLICATION(app), argc, argv);
    g_object_unref(app);
    return s;
}
