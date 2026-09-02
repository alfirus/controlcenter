project('controlcenter-terminal', 'c', version: '0.1.0')
gtk = dependency('gtk4')
vte = dependency('vte-2.91-gtk4', required: false)
executable('controlcenter-terminal', 'terminal.c', dependencies: [gtk, vte])
