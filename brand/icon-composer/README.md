# AppIcon import kit

`AppIcon.icon` is the completed Icon Composer document for this brand. It embeds the Default SVG as its editable vector source; Icon Composer automatically renders the Default, Dark, and Mono system appearances. The numeric SVG prefixes are the z-order from back to front and remain the editable source of truth.

To rebuild the document, import `../exports/measuretrail-app-icon-default.svg` as the base image. The matching Dark and Mono SVG/PNG exports are review references and optional future annotations; do not replace the shared base source solely to preview another appearance. Let Icon Composer apply the system enclosure mask and Liquid Glass properties. Do not add shadows, blur, gradients, or a rounded-square mask to these sources.

The flat 1024×1024 validation exports live in `../exports/`. This import kit is intentionally separate from the eventual Xcode project, which is created in Phase 5.
