import { fireEvent, render } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import CandidateCard from './CandidateCard.svelte';
import type { Candidate } from '$lib/api/types';

function makeCandidate(overrides: Partial<Candidate> = {}): Candidate {
	return {
		tmdb_id: 27205,
		kind: 'movie',
		title: 'Inception',
		year: 2010,
		score: 0.87,
		...overrides
	};
}

describe('CandidateCard', () => {
	it('muestra título, año, tipo y el score como porcentaje', () => {
		const { getByText } = render(CandidateCard, { candidate: makeCandidate(), onChoose: vi.fn() });
		expect(getByText('Inception')).toBeInTheDocument();
		expect(getByText('2010')).toBeInTheDocument();
		expect(getByText('Película')).toBeInTheDocument();
		expect(getByText('87%')).toBeInTheDocument();
	});

	it('kind=tv se muestra como "Serie"', () => {
		const { getByText } = render(CandidateCard, {
			candidate: makeCandidate({ kind: 'tv' }),
			onChoose: vi.fn()
		});
		expect(getByText('Serie')).toBeInTheDocument();
	});

	it('sin poster_path, cae a la inicial del título en vez de romper', () => {
		const { getByText, container } = render(CandidateCard, {
			candidate: makeCandidate({ poster_path: undefined }),
			onChoose: vi.fn()
		});
		expect(getByText('I')).toBeInTheDocument();
		expect(container.querySelector('img')).toBeNull();
	});

	it('al pulsar, llama a onChoose con el candidato', async () => {
		const onChoose = vi.fn();
		const candidate = makeCandidate();
		const { getByText } = render(CandidateCard, { candidate, onChoose });
		await fireEvent.click(getByText('Inception'));
		expect(onChoose).toHaveBeenCalledWith(candidate);
	});

	it('disabled=true no deja pulsar', async () => {
		const onChoose = vi.fn();
		const { container } = render(CandidateCard, { candidate: makeCandidate(), onChoose, disabled: true });
		const button = container.querySelector('button');
		expect(button).toBeDisabled();
	});
});
