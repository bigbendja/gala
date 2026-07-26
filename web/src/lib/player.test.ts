import { describe, expect, it } from 'vitest';
import { isPlayable, pickNextSource } from './player';
import type { PlaySourceView } from './api/types';

function src(overrides: Partial<PlaySourceView> = {}): PlaySourceView {
	return {
		source_id: 1,
		provider: 'localfs',
		health: 'viva',
		mode: 'direct',
		url: '/api/v1/stream/1',
		...overrides
	};
}

describe('isPlayable', () => {
	it('direct y remux son reproducibles', () => {
		expect(isPlayable('direct')).toBe(true);
		expect(isPlayable('remux')).toBe(true);
	});
	it('reject y analyzing no', () => {
		expect(isPlayable('reject')).toBe(false);
		expect(isPlayable('analyzing')).toBe(false);
	});
});

describe('pickNextSource', () => {
	it('elige la primera reproducible no excluida, respetando el orden dado', () => {
		const sources = [
			src({ source_id: 1, mode: 'direct' }),
			src({ source_id: 2, mode: 'remux' })
		];
		expect(pickNextSource(sources, new Set())?.source_id).toBe(1);
	});

	it('salta las ya excluidas (fuentes que ya fallaron esta sesión)', () => {
		const sources = [
			src({ source_id: 1, mode: 'direct' }),
			src({ source_id: 2, mode: 'remux' })
		];
		expect(pickNextSource(sources, new Set([1]))?.source_id).toBe(2);
	});

	it('nunca elige automáticamente reject/analyzing', () => {
		const sources = [
			src({ source_id: 1, mode: 'reject' }),
			src({ source_id: 2, mode: 'analyzing' }),
			src({ source_id: 3, mode: 'direct' })
		];
		expect(pickNextSource(sources, new Set())?.source_id).toBe(3);
	});

	it('devuelve null si no queda ninguna reproducible', () => {
		const sources = [src({ source_id: 1, mode: 'direct' })];
		expect(pickNextSource(sources, new Set([1]))).toBeNull();
	});
});
