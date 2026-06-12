#ifndef TRAY_DARWIN_H
#define TRAY_DARWIN_H

void tray_init(void);
void tray_set_port_count(int count);
void tray_set_port_label(int i, const char *s);
void tray_set_bat(const char *s);
void tray_set_total(const char *s);
void tray_set_title(const char *s);
void tray_run(double interval);
void tray_quit(void);

#endif
