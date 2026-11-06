// Tile classes shared by the Leaflet map and the list minimap. Kept free of Leaflet on purpose:
// the list must not pull the library into its bundle.

// OpenStreetMap only has a light style: in the dark theme, invert and darken the tiles.
export const TILE_LIGHT = "transition-opacity duration-(--dur-color) ease-value";
export const TILE_DARK =
  "transition-opacity duration-(--dur-color) ease-value [filter:invert(1)_hue-rotate(180deg)_brightness(0.9)_contrast(0.9)_saturate(0.6)]";
