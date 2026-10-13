<template>
  <div v-if="showNotice" class="reconnect-notice" role="status">
    <p v-if="byIP">You opened Reflash by IP address. This address may change when the printer joins Wi-Fi.</p>
    <p v-if="active">Rejoin {{ target ? `the ${target} network` : 'the destination Wi-Fi network' }} on this computer or phone, then open Reflash at its new address.</p>
    <p v-if="url">Reconnect using <a :href="url">{{ url }}</a>. The name may change if another device uses it.</p>
    <p v-else>The board's mDNS name is unavailable. Find its new address in your router's connected-device list.</p>
  </div>
</template>
<script>
import axios from 'axios';
import { isIPAddress, reconnectURL } from '../network';
export default {
  name: 'ReconnectNotice',
  props: { active: Boolean, target: String, transitionOnly: Boolean },
  data: () => ({ hostname: '', locationURL: window.location.href }),
  computed: {
    byIP() { return isIPAddress(new URL(this.locationURL).hostname); },
    showNotice() { return this.transitionOnly ? this.active : this.byIP || this.active; },
    url() { return reconnectURL(this.locationURL, this.hostname); },
  },
  async mounted() {
    try {
      const result = await axios.get('/api/reconnect_address', { timeout: 3000 });
      this.hostname = result.data.hostname || '';
    } catch (_) { this.hostname = ""; }
  },
};
</script>
<style scoped>
.reconnect-notice { padding: 8px 16px; border: 1px solid currentColor; margin: 8px 0; text-align: left; }
.reconnect-notice p { margin: 4px 0; }
.reconnect-notice a { text-decoration: underline; }
</style>
