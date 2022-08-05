import axios from 'axios';

const state = {
  options: {
    darkmode: true,
    rebootWhenDone: false,
    enableSsh: false,
    bootFromEmmc: false
  }
};
const getters = {
  options: (state) => state.options
};
const actions = {
  async getOptions({ commit }){
    const response = await axios.get(`/api/options`)
    commit('getOptions', response.data);
  },
  async setOption({ commit }, option){
    commit('setOption', option);
    await axios.post(`/api/save_options`, state.options);
  }
};
const mutations = {
  getOptions: (state, options) => (state.options = options),
  setOption: (state, option) => (state.options = {...state.options, ...option })
};
export default {
  state,
  getters,
  actions,
  mutations
};
