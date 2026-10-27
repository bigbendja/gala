import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import LanPanel from './LanPanel.svelte';
import PinGate from './PinGate.svelte';
import QrCode from './QrCode.svelte';
import Topbar from './Topbar.svelte';
import type { LanStatus } from '$lib/api/types';

const { lanAuthMock, getLanStatusMock, setLanEnabledMock, setLanPinMock } = vi.hoisted(() => ({
	lanAuthMock: vi.fn(),
	getLanStatusMock: vi.fn(),
	setLanEnabledMock: vi.fn(),
	setLanPinMock: vi.fn()
}));

vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return {
		...actual,
		lanAuth: lanAuthMock,
		getLanStatus: getLanStatusMock,
		setLanEnabled: setLanEnabledMock,
		setLanPin: setLanPinMock
	};
});

vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/') } }));

function status(over: Partial<LanStatus> = {}): LanStatus {
	return {
		enabled: false,
		bind: '127.0.0.1',
		port: 7420,
		lan_ip: '192.168.1.20',
		url: 'http://192.168.1.20:7420',
		pin_set: false,
		qr_payload: 'http://192.168.1.20:7420',
		candidates: [{ ip: '192.168.1.20', interface: 'Wi-Fi', virtual: false }],
		...over
	};
}

beforeEach(() => {
	lanAuthMock.mockReset();
	getLanStatusMock.mockReset();
	setLanEnabledMock.mockReset();
	setLanPinMock.mockReset();
});

describe('QrCode', () => {
	it('dibuja un SVG con módulos', () => {
		const { container } = render(QrCode, { value: 'http://192.168.1.20:7420' });
		const path = container.querySelector('svg path');
		expect(path?.getAttribute('d')?.length).toBeGreaterThan(100);
	});
});

describe('PinGate', () => {
	it('no deja enviar hasta tener 4 dígitos y filtra lo que no son números', async () => {
		render(PinGate, { onUnlocked: vi.fn() });
		const input = screen.getByLabelText('PIN de 4 dígitos') as HTMLInputElement;
		const go = screen.getByRole('button', { name: 'Entrar' }) as HTMLButtonElement;
		expect(go.disabled).toBe(true);
		await fireEvent.input(input, { target: { value: '12ab3' } });
		expect(input.value).toBe('123');
		expect(go.disabled).toBe(true);
		await fireEvent.input(input, { target: { value: '1234' } });
		expect(go.disabled).toBe(false);
	});

	it('PIN correcto: llama a onUnlocked', async () => {
		lanAuthMock.mockResolvedValue({ ok: true });
		const onUnlocked = vi.fn();
		render(PinGate, { onUnlocked });
		await fireEvent.input(screen.getByLabelText('PIN de 4 dígitos'), { target: { value: '2468' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Entrar' }));
		await waitFor(() => expect(onUnlocked).toHaveBeenCalled());
		expect(lanAuthMock).toHaveBeenCalledWith('2468');
	});

	it('PIN incorrecto: muestra el error y no entra', async () => {
		const { ApiError } = await import('$lib/api/client');
		lanAuthMock.mockRejectedValue(new ApiError('PIN incorrecto', 401));
		const onUnlocked = vi.fn();
		render(PinGate, { onUnlocked });
		await fireEvent.input(screen.getByLabelText('PIN de 4 dígitos'), { target: { value: '0000' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Entrar' }));
		expect((await screen.findByRole('alert')).textContent).toContain('PIN incorrecto');
		expect(onUnlocked).not.toHaveBeenCalled();
	});

	it('bloqueo por demasiados intentos: enseña el mensaje del servidor', async () => {
		const { ApiError } = await import('$lib/api/client');
		lanAuthMock.mockRejectedValue(new ApiError('Demasiados intentos. Espera 31 segundos.', 429));
		render(PinGate, { onUnlocked: vi.fn() });
		await fireEvent.input(screen.getByLabelText('PIN de 4 dígitos'), { target: { value: '1111' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Entrar' }));
		expect((await screen.findByRole('alert')).textContent).toContain('Espera 31 segundos');
	});
});

describe('LanPanel', () => {
	it('desactivado: pide confirmación con el aviso de seguridad antes de activar', async () => {
		getLanStatusMock.mockResolvedValue(status());
		setLanEnabledMock.mockResolvedValue(status({ enabled: true }));
		const onChanged = vi.fn();
		render(LanPanel, { open: true, onClose: vi.fn(), onChanged });

		await fireEvent.click(await screen.findByRole('button', { name: 'Activar Modo LAN' }));
		expect(setLanEnabledMock).not.toHaveBeenCalled(); // aún no: falta confirmar
		expect(screen.getByRole('alert').textContent).toContain('podrán ver tu galería');

		await fireEvent.click(screen.getByRole('button', { name: 'Entendido, activar' }));
		await waitFor(() => expect(setLanEnabledMock).toHaveBeenCalledWith(true));
		await waitFor(() => expect(onChanged).toHaveBeenCalledWith(true));
	});

	it('activo: enseña QR y URL (sin PIN en ellos) y permite desactivar', async () => {
		getLanStatusMock.mockResolvedValue(status({ enabled: true, pin_set: true }));
		setLanEnabledMock.mockResolvedValue(status({ enabled: false }));
		const onChanged = vi.fn();
		const { container } = render(LanPanel, { open: true, onClose: vi.fn(), onChanged });

		expect(await screen.findByText('http://192.168.1.20:7420')).toBeTruthy();
		expect(container.querySelector('svg.qr')).toBeTruthy();
		// El QR codifica solo la URL: el PIN no existe en el cliente (la API solo da pin_set).
		expect(container.textContent).toContain('Activado');

		await fireEvent.click(screen.getByRole('button', { name: 'Desactivar Modo LAN' }));
		await waitFor(() => expect(setLanEnabledMock).toHaveBeenCalledWith(false));
		await waitFor(() => expect(onChanged).toHaveBeenCalledWith(false));
	});

	it('PIN: valida 4 dígitos, lo guarda y se puede quitar', async () => {
		getLanStatusMock.mockResolvedValue(status({ enabled: true }));
		setLanPinMock.mockResolvedValue(status({ enabled: true, pin_set: true }));
		render(LanPanel, { open: true, onClose: vi.fn(), onChanged: vi.fn() });

		await fireEvent.click(await screen.findByRole('button', { name: 'Poner PIN' }));
		const save = screen.getByRole('button', { name: 'Guardar' }) as HTMLButtonElement;
		const input = screen.getByLabelText('PIN de 4 dígitos');
		await fireEvent.input(input, { target: { value: '12' } });
		expect(save.disabled).toBe(true);
		await fireEvent.input(input, { target: { value: '1357' } });
		expect(save.disabled).toBe(false);
		await fireEvent.click(save);
		await waitFor(() => expect(setLanPinMock).toHaveBeenCalledWith('1357'));

		setLanPinMock.mockResolvedValue(status({ enabled: true, pin_set: false }));
		await fireEvent.click(await screen.findByRole('button', { name: 'Quitar PIN' }));
		await waitFor(() => expect(setLanPinMock).toHaveBeenLastCalledWith(''));
	});

	it('sin red detectada: avisa en vez de enseñar un QR roto', async () => {
		getLanStatusMock.mockResolvedValue(
			status({ enabled: true, lan_ip: '', url: '', qr_payload: '', candidates: [] })
		);
		const { container } = render(LanPanel, { open: true, onClose: vi.fn(), onChanged: vi.fn() });
		expect(await screen.findByText('No encuentro tu red')).toBeTruthy();
		expect(container.querySelector('svg.qr')).toBeNull();
	});

	it('varias redes: permite elegir otra IP y el QR/URL la usan', async () => {
		getLanStatusMock.mockResolvedValue(
			status({
				enabled: true,
				candidates: [
					{ ip: '192.168.1.20', interface: 'Wi-Fi', virtual: false },
					{ ip: '10.0.0.5', interface: 'Ethernet', virtual: false }
				]
			})
		);
		render(LanPanel, { open: true, onClose: vi.fn(), onChanged: vi.fn() });
		await fireEvent.click(await screen.findByRole('button', { name: '10.0.0.5' }));
		expect(await screen.findByText('http://10.0.0.5:7420')).toBeTruthy();
	});
});

describe('Topbar según quién mira', () => {
	it('en el PC: Ajustes, Añadir y Modo LAN visibles', () => {
		render(Topbar, { onOpenIngest: vi.fn(), pendingCount: 2, local: true, lanEnabled: false });
		expect(screen.getByLabelText('Ajustes')).toBeTruthy();
		expect(screen.getByRole('button', { name: /Añadir/ })).toBeTruthy();
		expect(screen.getByRole('button', { name: /Modo LAN/ })).toBeTruthy();
		expect(screen.getByText('2 por confirmar')).toBeTruthy();
	});

	it('en un dispositivo de la red: nada de gestión', () => {
		render(Topbar, { onOpenIngest: vi.fn(), pendingCount: 2, local: false });
		expect(screen.queryByLabelText('Ajustes')).toBeNull();
		expect(screen.queryByRole('button', { name: /Añadir/ })).toBeNull();
		expect(screen.queryByRole('button', { name: /Modo LAN/ })).toBeNull();
		expect(screen.queryByText('2 por confirmar')).toBeNull();
	});

	it('la píldora refleja que el Modo LAN está activo', () => {
		render(Topbar, { onOpenIngest: vi.fn(), local: true, lanEnabled: true });
		expect(screen.getByRole('button', { name: /Modo LAN · activo/ })).toBeTruthy();
	});
});
