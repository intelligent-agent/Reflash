from setuptools import setup

setup(
   name='reflash',
   version='0.1',
   description='Server that can flash Refactor images',
   author='Elias Bakken',
   license="AGPLv3",
   packages=['reflash'],  #same as name
   install_requires=['wheel'], #external packages as dependencies
)
