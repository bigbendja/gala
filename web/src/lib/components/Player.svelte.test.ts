import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import Player from './Player.svelte';
import type { Item, PlayResponse } from '$lib/api/types';

const { reportSourceFailedMock, getPlayMock, putPlaybackMock } = vi.hoisted(() => ({
	reportSourceFailedMock: vi.fn().mockResolvedValue({ status: 'ok', health: 'caída' }),
	getPlayMock: vi.fn(),
	putPlaybackMock: vi.fn().mockResolvedValue({ status: 'ok', watched: false })
}));

vi.mock('$lib/api/client', () => ({
	reportSourceFailed: reportSourceFailedMock,
	getPlay: getPlayMock,
	putPlayback: putPlaybackMock,
	startHLS: vi.fn(),
	streamUrl: (id: number) => `/api/v1/stream/${id}`
}));

function makeItem(): Item {
	return {
		id: 1,
		kind: 'movie',
		title: 'Cielo de Harmattan',
		genres: [],
		confirmed: true,
		tmdb_id: 27205,
		wished: false,
		added_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
		source_count: 2
	};
}

function makePlay(): PlayResponse {
	return {
		item_id: 1,
		ffmpeg_available: true,
		chosen_source_id: 1,
		sources: [
			{ source_id: 1, provider: 'localfs', health: 'viva', mode: 'direct', url: '/api/v1/stream/1' },
			{ source_id: 2, provider: 'http', health: 'viva', mode: 'direct', url: '/api/v1/stream/2' }
		]
	};
}

describe('Player', () => {
	it('al fallar la fuente activa, avisa al backend y salta a la siguiente (Brief M10b §1)', async () => {
		const { container, getByText } = render(Player, {
			item: makeItem(),
			initialPlay: makePlay(),
			initialProgress: null
		});

		const video = container.querySelector('video');
		expect(video).toBeTruthy();
		expect(video?.getAttribute('src')).toContain('/api/v1/stream/1');

		await fireEvent.error(video as HTMLVideoElement);

		await waitFor(() => expect(reportSourceFailedMock).toHaveBeenCalledWith(1));
		await waitFor(() => expect(getByText('La fuente falló, probando la siguiente…')).toBeInTheDocument());

		const videoAfter = container.querySelector('video');
		expect(videoAfter?.getAttribute('src')).toContain('/api/v1/stream/2');
	});

	it('si fallan todas las fuentes, muestra el mensaje de agotadas', async () => {
		const play = makePlay();
		const { container, getByText } = render(Player, {
			item: makeItem(),
			initialPlay: play,
			initialProgress: null
		});

		await fireEvent.error(container.querySelector('video') as HTMLVideoElement);
		await waitFor(() => expect(container.querySelector('video')?.getAttribute('src')).toContain('/api/v1/stream/2'));
		await fireEvent.error(container.querySelector('video') as HTMLVideoElement);

		await waitFor(() =>
			expect(getByText('Ninguna fuente disponible pudo reproducirse.')).toBeInTheDocument()
		);
		expect(container.querySelector('video')).toBeNull();
	});

	it('una fuente reject muestra el motivo y el botón de copiar enlace, sin <video>', () => {
		const play: PlayResponse = {
			item_id: 1,
			ffmpeg_available: true,
			chosen_source_id: 1,
			sources: [
				{
					source_id: 1,
					provider: 'localfs',
					health: 'viva',
					mode: 'reject',
					url: '/api/v1/stream/1',
					reason: 'no se detectó ninguna pista de vídeo reproducible'
				}
			]
		};
		const { container, getByText } = render(Player, {
			item: makeItem(),
			initialPlay: play,
			initialProgress: null
		});
		expect(getByText('no se detectó ninguna pista de vídeo reproducible')).toBeInTheDocument();
		expect(getByText('Copiar enlace')).toBeInTheDocument();
		expect(container.querySelector('video')).toBeNull();
	});
});
