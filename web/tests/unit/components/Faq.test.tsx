import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Faq } from '../../../src/components/Faq';
import { FAQS } from '../../../src/data';

describe('Faq', () => {
  it('renders a details entry per FAQ with its question as a summary', () => {
    const { container } = render(<Faq />);
    expect(screen.getByRole('heading', { level: 2, name: /Frequently asked questions/ })).toBeInTheDocument();
    expect(container.querySelectorAll('details.faq').length).toBe(FAQS.length);
    expect(screen.getByText(FAQS[0][0])).toBeInTheDocument();
  });
});
