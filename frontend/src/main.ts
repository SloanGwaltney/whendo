import { mount } from 'svelte'
import './styles/theme.css'
import './styles/base.css'
import App from './App.svelte'

mount(App, { target: document.getElementById('app')! })
