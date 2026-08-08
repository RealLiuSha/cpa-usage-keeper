import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { BrandLink } from '../BrandLink';
import { GITHUB_REPOSITORY_URL } from '@/utils/constants';

describe('BrandLink', () => {
  it('renders the brand mark without linking to the GitHub repository', () => {
    const html = renderToStaticMarkup(<BrandLink />);

    expect(html).toContain('CPA Usage Keeper');
    expect(html).toContain('KEEPER');
    expect(html).toContain('aria-hidden="true"');
    expect(html).not.toContain('<a ');
    expect(html).not.toContain('href=');
    expect(html).not.toContain(GITHUB_REPOSITORY_URL);
    expect(html).not.toContain('target="_blank"');
  });
});
