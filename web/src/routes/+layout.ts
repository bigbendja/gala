// SPA pura: sin renderizado en servidor (CLAUDE.md M10a — "Cliente, sin
// SSR"). GALA es un binario que sirve esta build estática; no hay Node en
// producción, solo el propio backend Go respondiendo a /api/v1/*.
export const ssr = false;
