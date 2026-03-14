import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  integrations: [
    starlight({
      title: 'protomcp',
      description: 'Language-agnostic MCP runtime',
      social: {
        github: 'https://github.com/msilverblatt/protomcp',
      },
      sidebar: [
        { label: 'Demo', slug: 'demo', badge: { text: 'Live', variant: 'tip' } },
        {
          label: 'Getting Started',
          items: [
            { label: 'Installation', slug: 'getting-started/installation' },
            { label: 'Quick Start', slug: 'getting-started/quick-start' },
            { label: 'How It Works', slug: 'getting-started/how-it-works' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'Writing Tools (Python)', slug: 'guides/writing-tools-python' },
            { label: 'Writing Tools (TypeScript)', slug: 'guides/writing-tools-typescript' },
            { label: 'Writing Tools (Go)', slug: 'guides/writing-tools-go' },
            { label: 'Writing Tools (Rust)', slug: 'guides/writing-tools-rust' },
            { label: 'Resources', slug: 'guides/resources' },
            { label: 'Prompts & Completions', slug: 'guides/prompts' },
            { label: 'Sampling', slug: 'guides/sampling' },
            { label: 'Custom Middleware', slug: 'guides/middleware' },
            { label: 'Authentication', slug: 'guides/auth' },
            { label: 'Dynamic Tool Lists', slug: 'guides/dynamic-tool-lists' },
            { label: 'Hot Reload', slug: 'guides/hot-reload' },
            { label: 'Error Handling', slug: 'guides/error-handling' },
            { label: 'Production Deployment', slug: 'guides/production-deployment' },
            { label: 'Writing a Language Library', slug: 'guides/writing-a-language-library' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'CLI', slug: 'reference/cli' },
            { label: 'MCP Spec Compliance', slug: 'reference/mcp-compliance' },
            { label: 'Protobuf Spec', slug: 'reference/protobuf-spec' },
            { label: 'Python API', slug: 'reference/python-api' },
            { label: 'TypeScript API', slug: 'reference/typescript-api' },
          ],
        },
        {
          label: 'Concepts',
          items: [
            { label: 'Architecture', slug: 'concepts/architecture' },
            { label: 'Tool List Modes', slug: 'concepts/tool-list-modes' },
            { label: 'Transports', slug: 'concepts/transports' },
          ],
        },
      ],
    }),
  ],
});
