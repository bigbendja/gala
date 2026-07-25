import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';

// @testing-library/svelte no limpia el DOM entre tests por sí solo bajo
// Vitest (a diferencia de Jest, donde lo detecta automáticamente); sin esto,
// cada render() dentro del mismo archivo se acumula en document.body.
afterEach(() => cleanup());
