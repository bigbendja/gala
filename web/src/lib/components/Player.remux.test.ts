import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import Player from './Player.svelte';
import type { Item, PlayResponse } from '$lib/api/types';

const { startHLSMock, stopHLSMock, putPlaybackMock } = vi.hoisted(() => ({
	startHLSMock: vi.fn(),
	stopHLSMock: vi.fn(),
	putPlaybackMock: vi.fn().mockResolvedValue({ status: 'ok', watched: false })
}));

vi.mock('$lib/api/client', () => ({
	reportSourceFailed: vi.fn().mockResolvedValue({}),
	getPlay: vi.fn(),
	putPlayback: putPlaybackMock,
	startHLS: startHLSMock,
	stopHLS: stopHLSMock,
	streamUrl: (id: number) => `/api/v1/stream/${id}`
}));

// hls.js no hace nada en jsdom: solo importa que el reproductor lo monte y
// lo destruya una vez por sesión.
vi.mock('hls.js', () => {
	class FakeHls {
		static isSupported() {
			return true;
		}
		static Events = { ERROR: 'hlsError' };
		on() {}
		loadSource() {}
		attachMedia() {}
		destroy() {}
	}
	return { default: FakeHls };
});

function makeItem(): Item {
	return {
		id: 4,
		kind: 'movie',
		title: 'Origen',
		genres: [],
		confirmed: true,
		tmdb_id: 27205,
		wished: false,
		added_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
		source_count: 1
	};
}

function remuxPlay(): PlayResponse {
	return {
		item_id: 4,
		ffmpeg_available: true,
		chosen_source_id: 4,
		sources: [
			{ source_id: 4, provider: 'localfs', health: 'viva', mode: 'remux', url: '/api/v1/stream/4/hls/start' }
		]
	};
}

function setCurrentTime(video: HTMLVideoElement, t: number) {
	Object.defineProperty(video, 'currentTime', { value: t, configurable: true, writable: true });
}

describe('Player — remux', () => {
	it('arranca UNA sola sesión aunque el vídeo avance (antes: una por timeupdate, en bucle)', async () => {
		startHLSMock.mockReset().mockResolvedValue({ session_id: 's1', playlist_url: '/api/v1/stream/4/hls/s1/index.m3u8' });
		stopHLSMock.mockReset();

		const { container } = render(Player, { item: makeItem(), initialPlay: remuxPlay(), initialProgress: null });
		await waitFor(() => expect(startHLSMock).toHaveBeenCalledTimes(1));

		const video = container.querySelector('video') as HTMLVideoElement;
		for (const t of [0.2, 0.4, 0.8, 1.6, 3.2, 6.4]) {
			setCurrentTime(video, t);
			await fireEvent.timeUpdate(video);
		}
		await new Promise((r) => setTimeout(r, 50));

		expect(startHLSMock).toHaveBeenCalledTimes(1);
		expect(stopHLSMock).not.toHaveBeenCalled();
	});

	it('al salir del reproductor libera la sesión de remux en el servidor', async () => {
		startHLSMock.mockReset().mockResolvedValue({ session_id: 's7', playlist_url: '/api/v1/stream/4/hls/s7/index.m3u8' });
		stopHLSMock.mockReset();

		const { unmount } = render(Player, { item: makeItem(), initialPlay: remuxPlay(), initialProgress: null });
		await waitFor(() => expect(startHLSMock).toHaveBeenCalledTimes(1));
		await new Promise((r) => setTimeout(r, 20)); // deja que se registre la sesión tras el await

		unmount();
		expect(stopHLSMock).toHaveBeenCalledWith(4, 's7');
	});

	it('si se sale mientras la sesión todavía arrancaba, la libera en cuanto llega', async () => {
		let resolve!: (v: { session_id: string; playlist_url: string }) => void;
		startHLSMock.mockReset().mockReturnValue(new Promise((r) => (resolve = r)));
		stopHLSMock.mockReset();

		const { unmount } = render(Player, { item: makeItem(), initialPlay: remuxPlay(), initialProgress: null });
		await waitFor(() => expect(startHLSMock).toHaveBeenCalledTimes(1));
		unmount();
		resolve({ session_id: 'tarde', playlist_url: '/x' });

		await waitFor(() => expect(stopHLSMock).toHaveBeenCalledWith(4, 'tarde'));
	});
});
