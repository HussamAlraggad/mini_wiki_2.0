/**
 * Mini-wiki color theme.
 * Muted terminal palette — no neon, no emojis.
 * Follows AGENTS.md design rules.
 */
export const theme = {
  // Backgrounds
  bg: "#1a1a2e",           // Deep navy
  bgLighter: "#16213e",    // Lighter navy (input area, message blocks)
  bgDarker: "#12121e",     // Darkest (chat area)

  // Accents
  accent: "#4a6fa5",       // Muted blue (headers, branding)
  accentLight: "#6b8fc9",  // Lighter blue (source titles)

  // Text
  text: "#e0e0e0",         // Primary text
  textMuted: "#667788",    // Secondary text (info, sources, timestamps)

  // Status colors
  success: "#4caf50",      // AI messages, success states
  error: "#e53935",        // Error messages
  warning: "#ff9800",      // Warnings

  // UI chrome
  border: "#2a2a4e",       // Borders, dividers
}
