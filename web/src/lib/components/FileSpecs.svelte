<script lang="ts">
	// "Este archivo": ficha técnica de la mejor fuente analizada (media_info) y
	// cuándo se añadió. Solo lo que se sabe: una fila sin dato no se enseña.
	import type { MediaFile } from '$lib/api/types';
	import {
		audioCodecLabel,
		containerLabel,
		formatBytes,
		formatDate,
		formatRuntime,
		videoCodecLabel
	} from '$lib/format';

	interface Props {
		file: MediaFile | null;
		addedAt: string;
	}
	let { file, addedAt }: Props = $props();

	const rows = $derived.by(() => {
		const out: { role: string; value: string }[] = [];
		if (file) {
			if (file.width && file.height) out.push({ role: 'Resolución', value: `${file.width} × ${file.height}` });
			if (file.video_codec) out.push({ role: 'Vídeo', value: videoCodecLabel(file.video_codec) });
			if (file.audio_codec) out.push({ role: 'Audio', value: audioCodecLabel(file.audio_codec) });
			if (file.container) out.push({ role: 'Contenedor', value: containerLabel(file.container) });
			const dur = file.duration_s ? formatRuntime(Math.round(file.duration_s / 60)) : '';
			if (dur) out.push({ role: 'Duración', value: dur });
			if (file.size_bytes) out.push({ role: 'Tamaño', value: formatBytes(file.size_bytes) });
		}
		const added = formatDate(addedAt);
		if (added) out.push({ role: 'Añadida', value: added });
		return out;
	});
</script>

<div class="tech">
	{#each rows as row (row.role)}
		<div class="row"><span class="role">{row.role}</span><span class="val">{row.value}</span></div>
	{/each}
</div>

<style>
	.tech {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 4px 18px;
		box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
	}
	.row {
		display: flex;
		justify-content: space-between;
		gap: 16px;
		padding: 10px 0;
		border-bottom: 1px solid var(--line);
		font-size: 14px;
	}
	.row:last-child {
		border-bottom: none;
	}
	.role {
		color: var(--muted);
	}
	.val {
		font-weight: 500;
		text-align: right;
	}
</style>
