import axios from 'axios';

const state = {
  options: {    
  },
  progress: {
    install: {
      visible: false,
      progress: 0,
      bandwidth: 0,
      timeStarted: 0,
      timeFinished: 0
    },
    transfer: {
      visible: false,
      progress: 0,
      bandwidth: 0,
      timePassed: 0,
      timeRemaining: 0
    }
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
  async setOption({ commit }, option){
    commit('setOption', option);
    await axios.post(`/api/set_options`, state.options);
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
  setProgress: (state, { name, progress }) => (state.progress[name].progress = progress),
  setBandwidth: (state, { name, bandwidth }) => (state.progress[name].bandwidth = bandwidth),
  setVisible: (state, {name, visible}) => (state.progress[name].visible = visible),
  setTimeStarted: (state, { name, time }) => (state.progress[name].timeStarted = time),
  setTimeFinished: (state, {name, time}) => (state.progress[name].timeFinished = time),
  setFlashMethod: (state, payload) => (state.flash.selectedMethod = payload)
};
export default {
  state,
  getters,
  actions,
  mutations
};
