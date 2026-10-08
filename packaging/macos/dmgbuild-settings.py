# dmgbuild settings for the MyGO-Clash disk image; see make-dmg.sh.
# dmgbuild writes the Finder layout (.DS_Store) itself, so no Finder, AppleScript
# or GUI session is needed, which is what a CI runner lacks.
#
# Defines, passed by make-dmg.sh as -D name=value:
#   app   path of the built .app
#   here  directory holding background.png and background@2x.png
#   icon  optional path of the app's .icns, used as the volume icon
import os

app = defines["app"]  # noqa: F821  (defines is provided by dmgbuild)
here = defines["here"]  # noqa: F821
app_name = os.path.basename(app)

format = "ULMO"  # LZMA, like mygo's own images
filesystem = "HFS+"
files = [app]
symlinks = {"Applications": "/Applications"}
if defines.get("icon"):  # noqa: F821
    icon = defines["icon"]  # noqa: F821

# background.png and background@2x.png are merged into one HiDPI TIFF.
background = os.path.join(here, "background.png")

show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False

# Content size of the window; matches the background image in points.
window_rect = ((200, 120), (660, 400))
default_view = "icon-view"
icon_size = 128
text_size = 13
arrange_by = None
label_pos = "bottom"
icon_locations = {
    app_name: (180, 170),
    "Applications": (480, 170),
}
