import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

// The chat transport is not what these tests are about: stub @ai-sdk/react so
// the view's own states (empty, streaming, error) can be driven directly.
const chat = {
  messages: [] as unknown[],
  sendMessage: vi.fn(),
  status: 'ready' as string,
  error: undefined as Error | undefined,
  regenerate: vi.fn(),
  stop: vi.fn(),
  clearError: vi.fn(),
};

vi.mock('@ai-sdk/react', () => ({ useChat: () => chat }));

const { Ask, messageSources } = await import('../../../src/components/Ask');

describe('Ask', () => {
  beforeEach(() => {
    chat.messages = [];
    chat.status = 'ready';
    chat.error = undefined;
    chat.sendMessage.mockClear();
    chat.regenerate.mockClear();
    chat.stop.mockClear();
    chat.clearError.mockClear();
  });

  it('renders the hero, the conversation log and the composer', () => {
    const { container } = render(<Ask />);
    expect(container.querySelector('#view-ask')).not.toBeNull();
    expect(screen.getByRole('log', { name: 'Conversation' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: /Ask the go assistant/ })).toBeInTheDocument();
  });

  it('disables Send until the question is non-empty', async () => {
    render(<Ask />);
    const send = screen.getByRole('button', { name: 'Send' });
    expect(send).toBeDisabled();
    await userEvent.type(screen.getByRole('textbox'), 'how do I sign a JWT?');
    expect(send).toBeEnabled();
  });

  it('sends a question and clears the composer', async () => {
    render(<Ask />);
    await userEvent.type(screen.getByRole('textbox'), 'where is the HTML parser?');
    await userEvent.click(screen.getByRole('button', { name: 'Send' }));
    expect(chat.sendMessage).toHaveBeenCalledWith({ text: 'where is the HTML parser?' });
    expect(screen.getByRole('textbox')).toHaveValue('');
  });

  it('sends a suggestion when one is clicked', async () => {
    render(<Ask />);
    await userEvent.click(screen.getByRole('button', { name: 'How do I sign a JWT?' }));
    expect(chat.sendMessage).toHaveBeenCalledWith({ text: 'How do I sign a JWT?' });
  });

  it('surfaces a failed request with a retry, instead of failing silently', async () => {
    chat.error = new Error('rate limited');
    render(<Ask />);
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent(/couldn't answer/i);
    expect(alert).toHaveTextContent('rate limited');

    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(chat.regenerate).toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(chat.clearError).toHaveBeenCalled();
  });

  it('offers a Stop control only while a response is streaming', () => {
    const { rerender } = render(<Ask />);
    expect(screen.queryByRole('button', { name: 'Stop' })).toBeNull();

    chat.status = 'streaming';
    rerender(<Ask />);
    expect(screen.getByRole('button', { name: 'Stop' })).toBeInTheDocument();
  });

  it('does not send while a response is in flight', async () => {
    chat.status = 'streaming';
    render(<Ask />);
    // The composer is disabled, and even a suggestion click is a no-op.
    expect(screen.getByRole('button', { name: 'Sending…' })).toBeDisabled();
  });
});

describe('messageSources', () => {
  const hit = {
    name: 'Sign',
    kind: 'func',
    library: 'jwt',
    package: 'github.com/malcolmston/jwt',
    url: '/lib/jwt',
  };

  it('collects the completed searchSymbols results', () => {
    expect(
      messageSources([{ type: 'tool-searchSymbols', state: 'output-available', output: [hit] }]),
    ).toEqual([hit]);
  });

  it('ignores parts that are still running or are a different tool', () => {
    expect(
      messageSources([
        { type: 'tool-searchSymbols', state: 'input-available', output: [hit] },
        { type: 'tool-searchPastAnswers', state: 'output-available', output: [hit] },
        { type: 'text' },
      ]),
    ).toEqual([]);
  });

  it('dedupes by url', () => {
    expect(
      messageSources([
        { type: 'tool-searchSymbols', state: 'output-available', output: [hit, { ...hit }] },
      ]),
    ).toHaveLength(1);
  });

  it('drops malformed records rather than rendering junk', () => {
    // The tool output crosses a network boundary, so its shape is not typed at
    // runtime: anything missing a url/name/kind/library must be skipped.
    expect(
      messageSources([
        {
          type: 'tool-searchSymbols',
          state: 'output-available',
          output: [null, 'nope', { url: '' }, { url: '/x' }, hit],
        },
      ]),
    ).toEqual([hit]);
  });

  it('tolerates a non-array output', () => {
    expect(
      messageSources([{ type: 'tool-searchSymbols', state: 'output-available', output: 'oops' }]),
    ).toEqual([]);
  });
});
