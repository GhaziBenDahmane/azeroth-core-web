import { defineConfig } from 'astro/config';
import { webcore } from 'webcoreui/integration';

export default defineConfig({
  output: 'static',
  integrations: [webcore()],
  build: { assets: 'assets' },
});
