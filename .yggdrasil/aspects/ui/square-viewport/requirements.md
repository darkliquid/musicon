# Square viewport requirement

The user explicitly requires the Bubble Tea UI to scale to the current terminal dimensions while always fitting inside a square.

All screen composition must therefore treat the usable application frame as a centered `1:1` viewport derived from the smaller of terminal width and height.

Any leftover terminal space is outside the application frame and should be treated as padding rather than extra layout area.

The current Musicon implementation also enforces an explicit minimum terminal requirement of `20×20` so the square frame still has enough room for header, body, footer, and panel chrome without collapsing into unusable layouts.
