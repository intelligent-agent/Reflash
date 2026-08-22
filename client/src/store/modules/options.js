import axios from 'axios';

const state = {
  options: {    
  },
  progress: {
    progress: 0,
    bandwidth: 0,
    timeStarted: 0,
    timeFinished: 0
  },
  flash: {
    selectedMethod: 0
  }
};
const getters = {
  options: (state) => state.options,
  progress: (state) => state.progress,
  flash: (state) => state.flash
};
const actions = {
  async getOptions({ commit }){
    const response = await axios.get(`/api/get_options`)
    commit('getOptions', response.data);
  },
  // Post only what changed, never the whole cached object. The server merges
  // by unmarshalling onto its live struct, so a partial object leaves every
  // other field alone - including ones this client has never seen.
  //
  // Posting `state.options` was a lost update (#124). The page caches options
  // once at load and nothing re-fetches, while the server changes them
  // underneath: connecting to WiFi sets SSID and PSK server-side. So a page
  // loaded before that, then used to toggle rotation, wrote its stale empty
  // SSID back over working credentials - and the next flash produced an image
  // with no network, which looks like a flashing bug rather than a UI one.
  async setOption({ commit }, option){
    commit('setOption', option);
    await axios.post(`/api/set_options`, option);
  },
  setVisible({ commit }, payload){
    commit('setVisible', payload);
  },
  setProgress({ commit }, payload){
    commit('setProgress', payload);
  },
  setBandwidth({ commit }, payload){
    commit('setBandwidth', payload);
  },
  setTimeStarted({ commit }, payload){
    commit('setTimeStarted', payload);
  },
  setTimeFinished({ commit }, payload){
    commit('setTimeFinished', payload);
  },
  setFlashMethod({commit}, payload){
    commit('setFlashMethod', payload);
  }
};
const mutations = {
  getOptions: (state, options) => (state.options = options),
  setOption: (state, option) => (state.options = {...state.options, ...option }),
  setProgress: (state, { progress }) => (state.progress.progress = progress),
  setBandwidth: (state, {bandwidth }) => (state.progress.bandwidth = bandwidth),
  setTimeStarted: (state, { time }) => (state.progress.timeStarted = time),
  setTimeFinished: (state, {time}) => (state.progress.timeFinished = time),
  setFlashMethod: (state, payload) => (state.flash.selectedMethod = payload)
};
export default {
  state,
  getters,
  actions,
  mutations
};
