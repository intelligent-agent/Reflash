import axios from 'axios';

const state = {
  options: {
    darkmode: true,
    enableSsh: false,
  },
  bootMedia: "usb"
};
const getters = {
  options: (state) => state.options,
};
const actions = {
  async getOptions({ commit }){
    const response = await axios.get(`/api/options`)
    commit('getOptions', response.data);
  },
  async setOption({ commit }, option){
    commit('setOption', option);
    await axios.post(`/api/save_options`, state.options);
  },
  setBootMedia({ commit }, payload){
    commit('setBootMedia', payload);
  }
};
const mutations = {
  getOptions: (state, options) => (state.options = options),
  setOption: (state, option) => (state.options = {...state.options, ...option }),
  setBootMedia: (state, {boot_media}) => (state.options.bootMedia = boot_media),
};
export default {
  state,
  getters,
  actions,
  mutations
};
