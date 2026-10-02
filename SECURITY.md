# Security policy

## Supported versions

Only the latest release receives fixes.

## Reporting a vulnerability

Please do not open a public issue for a security problem.

Report it privately through GitHub: on the repository page open **Security → Report a vulnerability**
([direct link](https://github.com/kl09/mac-pulse/security/advisories/new)). Include the mac-pulse
version (Settings → General), the macOS version, and the steps to reproduce.

You can expect a first answer within a week. Once a fix is released, the advisory is published and
credits you unless you ask otherwise.

## What counts

mac-pulse runs as the logged-in user, without root, a helper or a listening socket. Reports that are
especially useful:

- a way to make mac-pulse remove, move or overwrite a file the user did not choose (the cleanup and export code);
- a way for a web page or another program to make mac-pulse do more than open a tab through the `mac-pulse://` link;
- a way to run code inside mac-pulse or its web view from data it displays (process names, tab titles, host names, Docker output);
- a network request the app makes without the button press the README describes.
